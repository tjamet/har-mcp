// Package har provides functionality for parsing and working with HAR (HTTP Archive) files.
package har

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/martian/har"
)

// Parser handles HAR file parsing from various sources. It holds the body
// store populated at parse time, so decoded response bodies can be fetched
// later by content hash, and the load policy that governed the most recent
// parse, so reads after the load apply the same exclusion decisions.
type Parser struct {
	bodies *BodyStore
	policy *LoadPolicy // set on each successful parse, alongside the store reset
}

// NewParser creates a new HAR parser
func NewParser() *Parser {
	return &Parser{bodies: NewBodyStore()}
}

// LoadPolicy controls which response bodies are stored at parse time.
// Bodies whose mime type matches an excluded prefix, or whose decoded size
// exceeds MaxKeepBytes, are NOT stored in the body store: they remain in the
// parsed HAR (details previews still work from the in-memory HAR) but get no
// body hash, so get_response_body cannot fetch them. MaxKeepBytes <= 0 means
// no size limit. Mime matching is a case-insensitive prefix match on the mime
// type before any ';' parameter, e.g. "video/" or "image/*" both match
// "video/mp4" / "image/jpeg".
type LoadPolicy struct {
	// ExcludeMimeTypes lists mime type prefixes whose bodies are not stored.
	ExcludeMimeTypes []string `json:"excludeMimeTypes,omitempty"`
	// MaxKeepBytes is the maximum decoded body size to store; larger bodies
	// are not stored. <= 0 means no limit.
	MaxKeepBytes int64 `json:"maxKeepBytes,omitempty"`
}

// excludes reports whether a body with the given mime type and decoded size
// is filtered out by the policy.
func (p *LoadPolicy) excludes(mimeType string, size int64) bool {
	if p.MaxKeepBytes > 0 && size > p.MaxKeepBytes {
		return true
	}
	mimeType = strings.ToLower(strings.SplitN(mimeType, ";", 2)[0])
	for _, pattern := range p.ExcludeMimeTypes {
		// "image/*" and "image/" are the same prefix. An empty pattern
		// ("" or "*") would match every mime type; skip it instead of
		// silently excluding all bodies.
		prefix := strings.ToLower(strings.TrimSuffix(pattern, "*"))
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(mimeType, prefix) {
			return true
		}
	}
	return false
}

// shouldStore reports whether a body should enter the body store under the
// current load policy. A nil policy stores everything.
func (p *Parser) shouldStore(mimeType string, size int64) bool {
	return p.policy == nil || !p.policy.excludes(mimeType, size)
}

// ParseFromFile parses a HAR file from disk
func (p *Parser) ParseFromFile(path string) (*har.HAR, error) {
	return p.ParseFromFileWithPolicy(path, nil)
}

// ParseFromFileWithPolicy is ParseFromFile with a load policy applied at
// index time.
func (p *Parser) ParseFromFileWithPolicy(path string, policy *LoadPolicy) (*har.HAR, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open HAR file: %w", err)
	}
	defer file.Close() //nolint:errcheck

	return p.ParseWithPolicy(file, policy)
}

// ParseFromURL parses a HAR file from an HTTP URL
func (p *Parser) ParseFromURL(harURL string) (*har.HAR, error) {
	return p.ParseFromURLWithPolicy(harURL, nil)
}

// ParseFromURLWithPolicy is ParseFromURL with a load policy applied at index
// time.
func (p *Parser) ParseFromURLWithPolicy(harURL string, policy *LoadPolicy) (*har.HAR, error) {
	resp, err := http.Get(harURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch HAR from URL: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch HAR: HTTP %d", resp.StatusCode)
	}

	return p.ParseWithPolicy(resp.Body, policy)
}

// Parse parses a HAR file from the given reader
func (p *Parser) Parse(r io.Reader) (*har.HAR, error) {
	return p.ParseWithPolicy(r, nil)
}

// ParseWithPolicy parses a HAR file from the given reader, applying the load
// policy when indexing response bodies.
func (p *Parser) ParseWithPolicy(r io.Reader, policy *LoadPolicy) (*har.HAR, error) {
	// Read all data so we can try multiple parsing approaches
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read HAR data: %w", err)
	}

	// First try standard parsing
	var harData har.HAR
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&harData); err == nil {
		// Standard parsing succeeded. The new load replaces the previous
		// one: reset policy and body store so the old HAR's bodies are no
		// longer fetchable.
		p.policy = policy
		p.bodies = NewBodyStore()
		p.indexBodies(&harData)
		return &harData, nil
	}

	// If standard parsing failed, try flexible parsing
	var flexibleHAR FlexibleHAR
	decoder = json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&flexibleHAR); err != nil {
		return nil, fmt.Errorf("failed to parse HAR file: %w", err)
	}

	// Convert flexible HAR to standard HAR
	standardHAR := flexibleHAR.ToStandardHAR()
	p.policy = policy
	p.bodies = NewBodyStore()
	p.indexBodies(standardHAR)
	return standardHAR, nil
}

// indexBodies stores every non-empty decoded response body and request
// postData allowed by the load policy in the body store, deduplicated by
// content hash. This is the single choke point where bodies enter the store,
// at parse time, covering both the standard decode and the FlexibleHAR
// fallback. Response bodies are indexed before postData within an entry, so
// when identical bytes appear in both, the response's mime type wins the
// first-store record. Reads never mutate the store: they look references up
// via BodyStore.Ref.
func (p *Parser) indexBodies(harData *har.HAR) {
	for _, entry := range harData.Log.Entries {
		if entry.Response != nil && entry.Response.Content != nil {
			content := entry.Response.Content
			if p.shouldStore(content.MimeType, int64(len(content.Text))) {
				p.bodies.Add(content.Text, content.MimeType)
			}
		}
		if entry.Request != nil && entry.Request.PostData != nil {
			pd := entry.Request.PostData
			if p.shouldStore(pd.MimeType, int64(len(pd.Text))) {
				p.bodies.Add([]byte(pd.Text), pd.MimeType)
			}
		}
	}
}

// URLMethodEntry represents a URL and method combination with associated request IDs
type URLMethodEntry struct {
	URL        string   `json:"url"`
	Method     string   `json:"method"`
	RequestIDs []string `json:"request_ids"`
}

// GetURLsAndMethods returns all unique URL and method combinations from the
// HAR. URLs are normalized (fragment stripped, sensitive query values
// redacted), matching the form shown by list_entries and get_request_details.
func (p *Parser) GetURLsAndMethods(harData *har.HAR) []URLMethodEntry {
	// Map to store unique URL+Method combinations and their request IDs
	urlMethodMap := make(map[string]*URLMethodEntry)

	for i, entry := range harData.Log.Entries {
		if entry.Request == nil {
			continue
		}

		url := normalizeURL(entry.Request.URL)
		key := fmt.Sprintf("%s|%s", url, entry.Request.Method)
		requestID := fmt.Sprintf("request_%d", i)

		if existing, ok := urlMethodMap[key]; ok {
			existing.RequestIDs = append(existing.RequestIDs, requestID)
		} else {
			urlMethodMap[key] = &URLMethodEntry{
				URL:        url,
				Method:     entry.Request.Method,
				RequestIDs: []string{requestID},
			}
		}
	}

	// Convert map to slice
	var result []URLMethodEntry
	for _, entry := range urlMethodMap {
		result = append(result, *entry)
	}

	return result
}

// GetRequestIDsForURLMethod returns all request IDs for a specific URL and method
func (p *Parser) GetRequestIDsForURLMethod(harData *har.HAR, targetURL, method string) []string {
	var requestIDs []string

	for i, entry := range harData.Log.Entries {
		if entry.Request == nil {
			continue
		}

		// Match on normalized URLs (fragment stripped, sensitive query
		// values redacted) so the form an agent copies from list_entries or
		// get_request_details round-trips into this query. A known tradeoff:
		// requests that differ only by a sensitive query value (e.g.
		// ?token=A vs ?token=B) normalize to the same form and collapse into
		// one match set.
		if normalizeURL(entry.Request.URL) == normalizeURL(targetURL) && entry.Request.Method == method {
			requestID := fmt.Sprintf("request_%d", i)
			requestIDs = append(requestIDs, requestID)
		}
	}

	return requestIDs
}

// EntrySummary is one compact row per HAR entry: the whole-file index view
// the agent sees in a single list_entries call. Deliberately flat and
// minimal — no headers, no startedDateTime (get_request_details has the full
// picture). URL is the normalized request URL (fragment stripped, sensitive
// query values redacted) capped at maxIndexURLLen; the full URL is in
// get_request_details.
type EntrySummary struct {
	RequestID string  `json:"requestId"` // positional "request_%d"
	Method    string  `json:"method"`
	URL       string  `json:"url"`    // normalized, capped at maxIndexURLLen
	Status    int     `json:"status"` // 0 when no response
	MimeType  string  `json:"mimeType,omitempty"`
	Size      int64   `json:"size,omitempty"` // decoded response body size
	TimeMs    float64 `json:"timeMs"`
	BodyHash  string  `json:"bodyHash,omitempty"` // body store ref, when a body was indexed
}

// GetEntries returns a page of compact per-entry rows in file order plus the
// total number of matching entries before pagination. filter is a
// case-insensitive substring match on the normalized URL — query params
// included, sensitive values redacted — so it never matches against the
// capped display form; method is an exact match; either filter is ignored
// when empty. Entries with a nil Request are skipped and do not count toward
// total. offset clamps to 0; limit defaults to 200 and caps at 1000. The
// returned page starts at offset and may be shorter than limit at the end of
// the match set.
func (p *Parser) GetEntries(harData *har.HAR, filter, method string, offset, limit int) ([]EntrySummary, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	var matches []EntrySummary
	for i, entry := range harData.Log.Entries {
		if entry.Request == nil {
			continue
		}
		url := normalizeURL(entry.Request.URL)
		if filter != "" && !strings.Contains(strings.ToLower(url), strings.ToLower(filter)) {
			continue
		}
		if method != "" && entry.Request.Method != method {
			continue
		}
		row := EntrySummary{
			RequestID: fmt.Sprintf("request_%d", i),
			Method:    entry.Request.Method,
			URL:       capURL(url),
			TimeMs:    float64(entry.Time),
		}
		if entry.Response != nil {
			row.Status = entry.Response.Status
			if entry.Response.Content != nil {
				row.MimeType = entry.Response.Content.MimeType
				row.Size = entry.Response.Content.Size
				// Lookup-only: bodies were indexed at parse time under the
				// load policy, so a body absent from the store was excluded
				// and gets no hash. Reads never mutate the store.
				row.BodyHash = p.bodies.Ref(entry.Response.Content.Text)
			}
		}
		matches = append(matches, row)
	}

	end := offset + limit
	if end > len(matches) {
		end = len(matches)
	}
	if offset > end {
		offset = end
	}
	return matches[offset:end], len(matches)
}

// sensitiveQueryKeys are query parameter names whose values are redacted
// from URLs and query strings. Header-name redaction cannot reach query
// strings, which commonly carry tokens (OAuth state, signed URLs, API keys).
// Bare "key" is deliberately absent: too generic, redacting every ?key=value
// would blind the index for ordinary lookups.
var sensitiveQueryKeys = map[string]bool{
	"access_token":  true,
	"api_key":       true,
	"apikey":        true,
	"auth":          true,
	"authorization": true,
	"jwt":           true,
	"password":      true,
	"passwd":        true,
	"pwd":           true,
	"secret":        true,
	"session":       true,
	"sessionid":     true,
	"sid":           true,
	"sig":           true,
	"signature":     true,
	"token":         true,
}

// normalizeURL returns a URL in canonical display and matching form: the
// fragment stripped and sensitive query values redacted. On parse error the
// raw input is returned unchanged. Matching (GetEntries filters,
// GetRequestIDsForURLMethod) compares normalized forms so what an agent
// copies from an index row or details call round-trips into queries.
func normalizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Fragment = ""
	if u.RawQuery != "" {
		u.RawQuery = redactRawQuery(u.RawQuery)
	}
	return u.String()
}

// redactRawQuery replaces the values of sensitive query parameters in a raw
// query string, preserving the order and escaping of non-sensitive pairs.
func redactRawQuery(rawQuery string) string {
	var kept []string
	for _, pair := range strings.Split(rawQuery, "&") {
		key, _, _ := strings.Cut(pair, "=")
		if decoded, err := url.QueryUnescape(key); err == nil && sensitiveQueryKeys[strings.ToLower(decoded)] {
			kept = append(kept, key+"=[REDACTED]")
		} else {
			kept = append(kept, pair)
		}
	}
	return strings.Join(kept, "&")
}

// redactQueryString redacts the values of sensitive query parameters in a
// har.QueryString slice, leaving everything else untouched.
func redactQueryString(qs []har.QueryString) []har.QueryString {
	out := make([]har.QueryString, len(qs))
	for i, q := range qs {
		out[i] = q
		if sensitiveQueryKeys[strings.ToLower(q.Name)] {
			out[i].Value = "[REDACTED]"
		}
	}
	return out
}

// redactCookies redacts the values of cookies whose names indicate auth
// material, so the cookies field does not leak what header redaction already
// hides (Set-Cookie / Cookie are redacted wholesale). A cookie counts as
// sensitive when its name matches a sensitiveQueryKeys entry or contains
// "session", "sessid", "token", or "auth" — covering session=, JSESSIONID,
// PHPSESSID, auth_token, and friends. Ordinary cookies (theme, lang, ...)
// keep their values; the cookie name itself is never a secret.
func redactCookies(cookies []har.Cookie) []har.Cookie {
	out := make([]har.Cookie, len(cookies))
	for i, c := range cookies {
		out[i] = c
		name := strings.ToLower(c.Name)
		if sensitiveQueryKeys[name] ||
			strings.Contains(name, "session") ||
			strings.Contains(name, "sessid") ||
			strings.Contains(name, "token") ||
			strings.Contains(name, "auth") {
			out[i].Value = "[REDACTED]"
		}
	}
	return out
}

// maxIndexURLLen caps the URL shown in a list_entries row. Query params are
// kept — they often discriminate requests (resource ids, pagination tokens) —
// but a row must stay compact, so longer URLs are cut at a rune boundary.
const maxIndexURLLen = 100

// capURL cuts s to at most maxIndexURLLen bytes (rune-safe) and appends an
// ellipsis when it was cut. Matching never runs against the capped form.
func capURL(s string) string {
	if len(s) <= maxIndexURLLen {
		return s
	}
	return cutRunes(s, maxIndexURLLen) + "…"
}

// RequestDetails represents the full details of a request with auth headers
// redacted and response bodies bounded to a preview. Raw bodies are never
// included: a huge response (e.g. a base64 mp4) would otherwise dump hundreds
// of thousands of tokens into the agent context on a single tool call.
type RequestDetails struct {
	RequestID       string        `json:"request_id"`
	StartedDateTime string        `json:"started_datetime"`
	Time            float64       `json:"time"`
	Request         *RequestInfo  `json:"request"`
	Response        *ResponseInfo `json:"response"`
	Cache           *har.Cache    `json:"cache,omitempty"`
	Timings         *har.Timings  `json:"timings,omitempty"`
	ServerIPAddress string        `json:"serverIPAddress,omitempty"`
	Connection      string        `json:"connection,omitempty"`
	Comment         string        `json:"comment,omitempty"`
}

// ResponseInfo is like har.Response but with redacted headers and a bounded
// body preview instead of the raw body.
type ResponseInfo struct {
	Status      int          `json:"status"`
	StatusText  string       `json:"statusText"`
	HTTPVersion string       `json:"httpVersion"`
	Cookies     []har.Cookie `json:"cookies"`
	Headers     []har.Header `json:"headers"`
	Content     *ContentInfo `json:"content"`
	RedirectURL string       `json:"redirectURL"`
	HeadersSize int64        `json:"headersSize"`
	BodySize    int64        `json:"bodySize"`
}

// ContentInfo is response content metadata with a bounded text preview. Binary
// content (video, images, fonts) carries metadata only — an LLM cannot read
// it, so shipping even a preview is token waste. Hash is the body-store
// reference for the full decoded body, fetched on demand via get_response_body.
type ContentInfo struct {
	Size        int64  `json:"size"`
	MimeType    string `json:"mimeType"`
	Encoding    string `json:"encoding,omitempty"`
	Hash        string `json:"hash,omitempty"`
	TextPreview string `json:"textPreview,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

// PostDataInfo is request post data with a bounded text preview. Binary
// uploads carry metadata only — an LLM cannot read them, so shipping even a
// preview is token waste. Hash is the body-store reference for the full
// post data, fetched on demand via get_response_body. Without this bound a
// large POST body (file upload, big form) would dump fully into the agent
// context on a single get_request_details call.
type PostDataInfo struct {
	MimeType    string      `json:"mimeType"`
	Params      []har.Param `json:"params,omitempty"`
	Size        int64       `json:"size"`
	Hash        string      `json:"hash,omitempty"`
	TextPreview string      `json:"textPreview,omitempty"`
	Truncated   bool        `json:"truncated,omitempty"`
}

// maxBodyPreview is the maximum number of body bytes included in a ContentInfo
// preview. This is what keeps get_request_details bounded regardless of the
// captured response size.
const maxBodyPreview = 4096

// truncationMarker is appended to a preview that was cut at maxBodyPreview.
// The cut can land mid-JSON, so the preview alone would look like a broken
// document; the footer makes the break explicit and points at the fetch tool.
const truncationMarker = "\n... [TRUNCATED — body continues past 4096 bytes; use get_response_body for the full content]"

// RequestInfo is like har.Request but with redacted auth headers and a
// bounded post-data preview instead of the raw body
type RequestInfo struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	HTTPVersion string            `json:"httpVersion"`
	Cookies     []har.Cookie      `json:"cookies"`
	Headers     []har.Header      `json:"headers"`
	QueryString []har.QueryString `json:"queryString"`
	PostData    *PostDataInfo     `json:"postData,omitempty"`
	HeadersSize int64             `json:"headersSize"`
	BodySize    int64             `json:"bodySize"`
}

// GetRequestDetails returns the full details of a request by ID with auth headers redacted
func (p *Parser) GetRequestDetails(harData *har.HAR, requestID string) (*RequestDetails, error) {
	// Extract index from request ID
	var index int
	if _, err := fmt.Sscanf(requestID, "request_%d", &index); err != nil {
		return nil, fmt.Errorf("invalid request ID format: %s", requestID)
	}

	if index < 0 || index >= len(harData.Log.Entries) {
		return nil, fmt.Errorf("request ID out of range: %s", requestID)
	}

	entry := harData.Log.Entries[index]

	// Entries with a nil Request are legal HAR (get_request_ids skips them)
	// but have no details to show.
	if entry.Request == nil {
		return nil, fmt.Errorf("request ID has no request data: %s", requestID)
	}

	// Create request info with redacted headers, cookie values, and query values
	requestInfo := &RequestInfo{
		Method:      entry.Request.Method,
		URL:         normalizeURL(entry.Request.URL),
		HTTPVersion: entry.Request.HTTPVersion,
		Cookies:     redactCookies(entry.Request.Cookies),
		Headers:     p.redactAuthHeaders(entry.Request.Headers),
		QueryString: redactQueryString(entry.Request.QueryString),
		PostData:    p.buildPostDataInfo(entry.Request.PostData),
		HeadersSize: entry.Request.HeadersSize,
		BodySize:    entry.Request.BodySize,
	}

	details := &RequestDetails{
		RequestID:       requestID,
		StartedDateTime: entry.StartedDateTime.Format(time.RFC3339),
		Time:            float64(entry.Time),
		Request:         requestInfo,
		Response:        p.buildResponseInfo(entry.Response),
		Cache:           entry.Cache,
		Timings:         entry.Timings,
	}

	return details, nil
}

// buildResponseInfo converts a har.Response into a ResponseInfo with redacted
// headers and a bounded text preview instead of the raw body.
func (p *Parser) buildResponseInfo(resp *har.Response) *ResponseInfo {
	if resp == nil {
		return nil
	}

	info := &ResponseInfo{
		Status:      resp.Status,
		StatusText:  resp.StatusText,
		HTTPVersion: resp.HTTPVersion,
		Cookies:     redactCookies(resp.Cookies),
		Headers:     p.redactAuthHeaders(resp.Headers),
		RedirectURL: resp.RedirectURL,
		HeadersSize: resp.HeadersSize,
		BodySize:    resp.BodySize,
	}

	if resp.Content != nil {
		info.Content = &ContentInfo{
			Size:     resp.Content.Size,
			MimeType: resp.Content.MimeType,
			Encoding: resp.Content.Encoding,
		}
		// Lookup-only: bodies were indexed at parse time under the load
		// policy. Bodies the policy excluded are absent from the store and
		// get no hash, so get_response_body cannot fetch them.
		info.Content.Hash = p.bodies.Ref(resp.Content.Text)
		if bodyReadable(resp.Content.MimeType, resp.Content.Text) && len(resp.Content.Text) > 0 {
			preview := string(resp.Content.Text)
			if len(preview) > maxBodyPreview {
				preview = cutRunes(preview, maxBodyPreview) + truncationMarker
				info.Content.Truncated = true
			}
			info.Content.TextPreview = preview
		}
	}

	return info
}

// buildPostDataInfo converts a har.PostData into a PostDataInfo with a
// bounded text preview instead of the raw body. postData.Text is used as-is
// (no base64 decoding): martian stores binary uploads as their base64 JSON
// form, which is fine — the hash is deterministic and dedup still works.
func (p *Parser) buildPostDataInfo(pd *har.PostData) *PostDataInfo {
	if pd == nil {
		return nil
	}

	info := &PostDataInfo{
		MimeType: pd.MimeType,
		Params:   pd.Params,
		Size:     int64(len(pd.Text)),
	}
	// Lookup-only: postData was indexed at parse time under the load
	// policy, so an excluded body is absent from the store and gets no hash.
	info.Hash = p.bodies.Ref([]byte(pd.Text))
	if bodyReadable(pd.MimeType, []byte(pd.Text)) && len(pd.Text) > 0 {
		preview := pd.Text
		if len(preview) > maxBodyPreview {
			preview = cutRunes(preview, maxBodyPreview) + truncationMarker
			info.Truncated = true
		}
		info.TextPreview = preview
	}

	return info
}

// isTextMime reports whether a mime type is likely to carry LLM-readable text.
// Binary mimes (video/*, image/*, font/*, ...) get metadata only.
func isTextMime(mime string) bool {
	mime = strings.ToLower(strings.SplitN(mime, ";", 2)[0])
	return strings.HasPrefix(mime, "text/") ||
		mime == "application/json" ||
		mime == "application/xml" ||
		mime == "application/javascript" ||
		mime == "application/x-javascript" ||
		mime == "application/x-www-form-urlencoded" ||
		strings.HasSuffix(mime, "+json") ||
		strings.HasSuffix(mime, "+xml")
}

// textSniffSample is how many leading bytes are inspected when the declared
// mime type does not identify a body as text.
const textSniffSample = 512

// looksLikeText is a cheap content sniff for bodies whose declared mime type
// is missing or wrong (e.g. JSON served as application/octet-stream). A body
// counts as text when a leading sample is valid UTF-8 with no C0 control
// bytes: text, JSON, HTML, JS all pass; decoded binary fails on its first
// NUL. It is a heuristic, not a verdict: base64-ascii payloads pass, which is
// fine — the preview is bounded either way.
func looksLikeText(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	sample := content
	if len(sample) > textSniffSample {
		sample = sample[:textSniffSample]
	}
	if !utf8.Valid(sample) {
		return false
	}
	for _, b := range sample {
		if b < 0x20 && b != '	' && b != '\n' && b != '\r' && b != '\f' {
			return false
		}
		if b == 0x7f {
			return false
		}
	}
	return true
}

// bodyReadable reports whether a body deserves a text preview: either the
// declared mime says text, or the bytes themselves look like text. Bodies
// that fail both are treated as binary and get metadata only.
func bodyReadable(mime string, content []byte) bool {
	return isTextMime(mime) || looksLikeText(content)
}

// cutRunes truncates s to at most max bytes without splitting a UTF-8 rune.
// Slicing mid-rune yields invalid UTF-8 that json.Marshal would silently
// replace with U+FFFD, corrupting the tail of every truncated preview.
func cutRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

// redactAuthHeaders redacts sensitive authentication headers
func (p *Parser) redactAuthHeaders(headers []har.Header) []har.Header {
	authHeaders := map[string]bool{
		"authorization":       true,
		"x-api-key":           true,
		"x-auth-token":        true,
		"cookie":              true,
		"set-cookie":          true,
		"proxy-authorization": true,
	}

	redactedHeaders := make([]har.Header, len(headers))
	for i, header := range headers {
		redactedHeaders[i] = har.Header{
			Name:  header.Name,
			Value: header.Value,
		}

		if authHeaders[strings.ToLower(header.Name)] {
			redactedHeaders[i].Value = "[REDACTED]"
		}
	}

	return redactedHeaders
}

// ParseSource parses a HAR file from either a file path or URL
func (p *Parser) ParseSource(source string) (*har.HAR, error) {
	return p.ParseSourceWithPolicy(source, nil)
}

// ParseSourceWithPolicy is ParseSource with a load policy applied at index
// time.
func (p *Parser) ParseSourceWithPolicy(source string, policy *LoadPolicy) (*har.HAR, error) {
	// Check if it's a URL
	if u, err := url.Parse(source); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return p.ParseFromURLWithPolicy(source, policy)
	}

	// Otherwise treat as file path
	return p.ParseFromFileWithPolicy(source, policy)
}

// BodyChunk is a bounded slice of a stored response body. Text bodies carry
// the chunk in Text; binary bodies carry metadata and a Note instead, because
// the bytes are not LLM-readable.
type BodyChunk struct {
	Hash      string `json:"hash"`
	Offset    int    `json:"offset"`
	Limit     int    `json:"limit"`
	TotalSize int64  `json:"totalSize"`
	Truncated bool   `json:"truncated"`
	Text      string `json:"text,omitempty"`
	MimeType  string `json:"mimeType"`
	Note      string `json:"note,omitempty"`
}

// defaultBodyChunk is the chunk size used when the caller omits limit.
const defaultBodyChunk = 4096

// maxBodyChunk caps a single chunk so a fetch cannot dump an unbounded body
// into the agent context.
const maxBodyChunk = 64 * 1024

// GetResponseBody returns a chunk of a stored response body between offset
// and offset+limit. Offset is clamped to 0; a missing or negative limit
// defaults to defaultBodyChunk and is capped at maxBodyChunk. Binary bodies
// return metadata only — the bytes are never shipped.
func (p *Parser) GetResponseBody(ref string, offset, limit int) (*BodyChunk, error) {
	body, totalSize, mime, ok := p.bodies.Get(ref)
	if !ok {
		return nil, fmt.Errorf("unknown body hash: %s", ref)
	}

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultBodyChunk
	}
	if limit > maxBodyChunk {
		limit = maxBodyChunk
	}

	chunk := &BodyChunk{
		Hash:      ref,
		Offset:    offset,
		Limit:     limit,
		TotalSize: totalSize,
		MimeType:  mime,
	}

	if !bodyReadable(mime, body) {
		chunk.Note = "binary body, not readable — metadata only"
		return chunk, nil
	}

	start := offset
	if start > len(body) {
		start = len(body)
	}
	end := start + limit
	if end > len(body) {
		end = len(body)
	}
	chunk.Text = string(body[start:end])
	chunk.Truncated = end < len(body)
	return chunk, nil
}
