package har

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/martian/har"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers

// createTestHAR creates a minimal valid HAR JSON for testing.
func createTestHAR() string {
	return `{
		"log": {
			"version": "1.2",
			"creator": {
				"name": "test-creator",
				"version": "1.0"
			},
			"entries": [
				{
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"time": 100,
					"request": {
						"method": "GET",
						"url": "https://example.com",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [
							{"name": "User-Agent", "value": "Test"},
							{"name": "Authorization", "value": "Bearer token123"}
						],
						"queryString": [],
						"headersSize": 150,
						"bodySize": 0
					},
					"response": {
						"status": 200,
						"statusText": "OK",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"content": {
							"size": 1024,
							"mimeType": "text/html"
						},
						"redirectURL": "",
						"headersSize": 200,
						"bodySize": 1024
					},
					"cache": {},
					"timings": {
						"blocked": 1,
						"dns": 2,
						"connect": 3,
						"send": 4,
						"wait": 50,
						"receive": 40,
						"ssl": 5
					}
				}
			]
		}
	}`
}

// createMultipleEntriesHAR creates a HAR with multiple entries
func createMultipleEntriesHAR() string {
	return `{
		"log": {
			"version": "1.2",
			"creator": {
				"name": "test-creator",
				"version": "1.0"
			},
			"entries": [
				{
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"time": 100,
					"request": {
						"method": "GET",
						"url": "https://example.com/api/users",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"queryString": [],
						"headersSize": 150,
						"bodySize": 0
					},
					"response": {
						"status": 200,
						"statusText": "OK",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"content": {
							"size": 1024,
							"mimeType": "application/json"
						},
						"redirectURL": "",
						"headersSize": 200,
						"bodySize": 1024
					}
				},
				{
					"startedDateTime": "2023-01-01T00:00:01.000Z",
					"time": 150,
					"request": {
						"method": "POST",
						"url": "https://example.com/api/users",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"queryString": [],
						"headersSize": 200,
						"bodySize": 50
					},
					"response": {
						"status": 201,
						"statusText": "Created",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"content": {
							"size": 512,
							"mimeType": "application/json"
						},
						"redirectURL": "",
						"headersSize": 180,
						"bodySize": 512
					}
				},
				{
					"startedDateTime": "2023-01-01T00:00:02.000Z",
					"time": 120,
					"request": {
						"method": "GET",
						"url": "https://example.com/api/users",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"queryString": [],
						"headersSize": 150,
						"bodySize": 0
					},
					"response": {
						"status": 200,
						"statusText": "OK",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"content": {
							"size": 2048,
							"mimeType": "application/json"
						},
						"redirectURL": "",
						"headersSize": 200,
						"bodySize": 2048
					}
				}
			]
		}
	}`
}

// createEmptyHAR creates a HAR with no entries.
func createEmptyHAR() string {
	return `{
		"log": {
			"version": "1.2",
			"creator": {
				"name": "test-creator",
				"version": "1.0"
			},
			"entries": []
		}
	}`
}

// parseTestHAR is a helper that parses test HAR data and requires success.
func parseTestHAR(t *testing.T, harData string) *har.HAR {
	t.Helper()

	parser := NewParser()
	reader := strings.NewReader(harData)
	archive, err := parser.Parse(reader)
	require.NoError(t, err, "failed to parse test HAR data")
	require.NotNil(t, archive, "parsed archive should not be nil")

	return archive
}

// Tests

func TestParseValidHAR(t *testing.T) {
	harData := createTestHAR()
	archive := parseTestHAR(t, harData)

	assert.Equal(t, "1.2", archive.Log.Version)
	assert.Equal(t, "test-creator", archive.Log.Creator.Name)
	assert.Equal(t, "1.0", archive.Log.Creator.Version)
	assert.Len(t, archive.Log.Entries, 1)

	// Check first entry
	entry := archive.Log.Entries[0]
	assert.Equal(t, "GET", entry.Request.Method)
	assert.Equal(t, "https://example.com", entry.Request.URL)
	assert.Equal(t, int64(100), entry.Time)
}

func TestParseEmptyEntries(t *testing.T) {
	harData := createEmptyHAR()
	archive := parseTestHAR(t, harData)

	assert.Equal(t, "1.2", archive.Log.Version)
	assert.Empty(t, archive.Log.Entries)
}

func TestParseInvalidJSON(t *testing.T) {
	invalidJSON := `{"log": invalid}`
	parser := NewParser()
	reader := strings.NewReader(invalidJSON)

	archive, err := parser.Parse(reader)

	assert.Error(t, err)
	assert.Nil(t, archive)
	assert.Contains(t, err.Error(), "failed to parse HAR file")
}

func TestGetURLsAndMethods(t *testing.T) {
	harData := createMultipleEntriesHAR()
	parser := NewParser()
	archive := parseTestHAR(t, harData)

	urlMethods := parser.GetURLsAndMethods(archive)

	assert.Len(t, urlMethods, 2) // GET and POST for /api/users

	// Find the GET entry
	var getEntry *URLMethodEntry
	for i := range urlMethods {
		if urlMethods[i].Method == "GET" {
			getEntry = &urlMethods[i]
			break
		}
	}

	require.NotNil(t, getEntry)
	assert.Equal(t, "https://example.com/api/users", getEntry.URL)
	assert.Equal(t, "GET", getEntry.Method)
	assert.Len(t, getEntry.RequestIDs, 2) // Two GET requests
}

func TestGetURLsAndMethodsRedactsSensitiveQueryValues(t *testing.T) {
	harData := fmt.Sprintf(`{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [{
				"startedDateTime": "2023-01-01T00:00:00.000Z",
				"time": 100,
				"request": {"method": "GET", "url": %q, "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "queryString": [], "headersSize": 150, "bodySize": 0},
				"response": null
			}]
		}
	}`, "https://example.com/api/data?token=supersecret&id=42")
	archive := parseTestHAR(t, harData)

	urlMethods := NewParser().GetURLsAndMethods(archive)

	require.Len(t, urlMethods, 1)
	assert.Contains(t, urlMethods[0].URL, "token=[REDACTED]")
	assert.NotContains(t, urlMethods[0].URL, "supersecret")
	assert.Contains(t, urlMethods[0].URL, "id=42")
}

func TestGetRequestIDsForURLMethod(t *testing.T) {
	harData := createMultipleEntriesHAR()
	parser := NewParser()
	archive := parseTestHAR(t, harData)

	// Test GET requests
	getIDs := parser.GetRequestIDsForURLMethod(archive, "https://example.com/api/users", "GET")
	assert.Len(t, getIDs, 2)
	assert.Contains(t, getIDs, "request_0")
	assert.Contains(t, getIDs, "request_2")

	// Test POST request
	postIDs := parser.GetRequestIDsForURLMethod(archive, "https://example.com/api/users", "POST")
	assert.Len(t, postIDs, 1)
	assert.Contains(t, postIDs, "request_1")

	// Test non-existent combination
	deleteIDs := parser.GetRequestIDsForURLMethod(archive, "https://example.com/api/users", "DELETE")
	assert.Empty(t, deleteIDs)
}

func TestGetRequestDetails(t *testing.T) {
	harData := createTestHAR()
	parser := NewParser()
	archive := parseTestHAR(t, harData)

	details, err := parser.GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details)

	assert.Equal(t, "request_0", details.RequestID)
	assert.Equal(t, float64(100), details.Time)

	// Check request details
	assert.Equal(t, "GET", details.Request.Method)
	assert.Equal(t, "https://example.com", details.Request.URL)
	assert.Equal(t, "HTTP/1.1", details.Request.HTTPVersion)

	// Check that auth header is redacted
	var authHeader *har.Header
	for i := range details.Request.Headers {
		if details.Request.Headers[i].Name == "Authorization" {
			authHeader = &details.Request.Headers[i]
			break
		}
	}
	require.NotNil(t, authHeader)
	assert.Equal(t, "[REDACTED]", authHeader.Value)

	// Check that non-auth header is not redacted
	var userAgentHeader *har.Header
	for i := range details.Request.Headers {
		if details.Request.Headers[i].Name == "User-Agent" {
			userAgentHeader = &details.Request.Headers[i]
			break
		}
	}
	require.NotNil(t, userAgentHeader)
	assert.Equal(t, "Test", userAgentHeader.Value)
}

// createResponseHAR creates a HAR with a single entry whose response has the
// given headers, mime type and body text, parsed with a fresh parser. The
// body is emitted base64-encoded because Go's encoding/json base64-decodes
// JSON strings into []byte fields.
func createResponseHAR(t *testing.T, headers []har.Header, mimeType, body string) *har.HAR {
	t.Helper()
	return createResponseHARWithParser(t, NewParser(), headers, mimeType, body)
}

// createResponseHARWithParser is createResponseHAR but parsed with the given
// parser, so the body lands in that parser's body store at parse time.
func createResponseHARWithParser(t *testing.T, parser *Parser, headers []har.Header, mimeType, body string) *har.HAR {
	t.Helper()
	return createResponseHARWithPolicy(t, parser, nil, headers, mimeType, body)
}

// createResponseHARWithPolicy is createResponseHARWithParser but parsed with
// the given load policy, so the policy governs which bodies land in the
// parser's body store.
func createResponseHARWithPolicy(t *testing.T, parser *Parser, policy *LoadPolicy, headers []har.Header, mimeType, body string) *har.HAR {
	t.Helper()

	headerJSON, err := json.Marshal(headers)
	require.NoError(t, err, "failed to marshal test headers")

	harData := fmt.Sprintf(`{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [
				{
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"time": 100,
					"request": {
						"method": "GET",
						"url": "https://example.com",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"queryString": [],
						"headersSize": 150,
						"bodySize": 0
					},
					"response": {
						"status": 200,
						"statusText": "OK",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": %s,
						"content": {
							"size": %d,
							"mimeType": %q,
							"text": %q
						},
						"redirectURL": "",
						"headersSize": 200,
						"bodySize": %d
					}
				}
			]
		}
	}`, string(headerJSON), len(body), mimeType, base64.StdEncoding.EncodeToString([]byte(body)), len(body))

	archive, err := parser.ParseWithPolicy(strings.NewReader(harData), policy)
	require.NoError(t, err, "failed to parse test HAR data")
	require.NotNil(t, archive, "parsed archive should not be nil")

	return archive
}

// createPostDataHAR creates a HAR with a single entry whose request carries
// the given postData mime type and body, parsed with the given parser under
// the given load policy. Post data text is emitted as a plain JSON string
// (no base64), matching how martian parses request bodies.
func createPostDataHAR(t *testing.T, parser *Parser, policy *LoadPolicy, mimeType, body string) *har.HAR {
	t.Helper()

	harData := fmt.Sprintf(`{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [
				{
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"time": 100,
					"request": {
						"method": "POST",
						"url": "https://example.com",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"queryString": [],
						"postData": {
							"mimeType": %q,
							"text": %q
						},
						"headersSize": 150,
						"bodySize": %d
					},
					"response": {
						"status": 200,
						"statusText": "OK",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"content": {"size": 0, "mimeType": "text/plain"},
						"redirectURL": "",
						"headersSize": 200,
						"bodySize": 0
					}
				}
			]
		}
	}`, mimeType, body, len(body))

	archive, err := parser.ParseWithPolicy(strings.NewReader(harData), policy)
	require.NoError(t, err, "failed to parse test HAR data")
	require.NotNil(t, archive, "parsed archive should not be nil")

	return archive
}

func TestGetRequestDetailsRedactsResponseHeaders(t *testing.T) {
	archive := createResponseHAR(t, []har.Header{
		{Name: "Content-Type", Value: "text/html"},
		{Name: "Set-Cookie", Value: "session=secret"},
	}, "text/html", "<html>ok</html>")

	details, err := NewParser().GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Response)
	for _, header := range details.Response.Headers {
		switch header.Name {
		case "Content-Type":
			assert.Equal(t, "text/html", header.Value)
		case "Set-Cookie":
			assert.Equal(t, "[REDACTED]", header.Value)
		}
	}
}

func TestGetRequestDetailsTruncatesLargeBodyPreview(t *testing.T) {
	body := strings.Repeat("a", maxBodyPreview+100)
	archive := createResponseHAR(t, nil, "text/plain", body)

	details, err := NewParser().GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Response.Content)
	assert.True(t, details.Response.Content.Truncated)
	// The preview carries maxBodyPreview body bytes plus the truncation
	// marker, which makes the break explicit instead of mid-JSON silence.
	assert.Len(t, details.Response.Content.TextPreview, maxBodyPreview+len(truncationMarker))
	assert.True(t, strings.HasSuffix(details.Response.Content.TextPreview, truncationMarker))
	assert.Equal(t, int64(len(body)), details.Response.Content.Size)
}

func TestGetRequestDetailsBinaryBodyMetadataOnly(t *testing.T) {
	body := "\x00\x01\x02\x03binary-garbage" // NUL bytes: never looks like text
	archive := createResponseHAR(t, nil, "video/mp4", body)

	details, err := NewParser().GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Response.Content)
	assert.Empty(t, details.Response.Content.TextPreview)
	assert.False(t, details.Response.Content.Truncated)
	assert.Equal(t, "video/mp4", details.Response.Content.MimeType)
}

func TestGetRequestDetailsPostDataHashPreviewAndFetch(t *testing.T) {
	parser := NewParser()
	body := `{"data": "test"}`
	archive := createPostDataHAR(t, parser, nil, "application/json", body)

	details, err := parser.GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Request.PostData)
	assert.Equal(t, "application/json", details.Request.PostData.MimeType)
	assert.Equal(t, int64(len(body)), details.Request.PostData.Size)
	sum := sha256.Sum256([]byte(body))
	assert.Equal(t, fmt.Sprintf("body:%x", sum), details.Request.PostData.Hash)
	assert.Equal(t, body, details.Request.PostData.TextPreview)
	assert.False(t, details.Request.PostData.Truncated)

	// The request body is fetchable chunked from the same body store.
	chunk, err := parser.GetResponseBody(details.Request.PostData.Hash, 0, 4096)
	require.NoError(t, err)
	assert.Equal(t, body, chunk.Text)
}

func TestPostDataIndexedAtParseTime(t *testing.T) {
	parser := NewParser()
	body := `{"data": "test"}`
	archive := createPostDataHAR(t, parser, nil, "application/json", body)

	// The postData is in the store from the parse alone — no details call
	// needed to make it fetchable.
	sum := sha256.Sum256([]byte(body))
	ref := fmt.Sprintf("body:%x", sum)
	chunk, err := parser.GetResponseBody(ref, 0, 4096)
	require.NoError(t, err)
	assert.Equal(t, body, chunk.Text)

	// Reads never mutate the store: querying the same archive through a
	// fresh parser (empty store) yields no hash for its bodies.
	details, err := NewParser().GetRequestDetails(archive, "request_0")
	require.NoError(t, err)
	require.NotNil(t, details.Request.PostData)
	assert.Empty(t, details.Request.PostData.Hash)
}

func TestGetRequestDetailsTruncatesLargePostDataPreview(t *testing.T) {
	body := strings.Repeat("a", maxBodyPreview+100)
	archive := createPostDataHAR(t, NewParser(), nil, "text/plain", body)

	details, err := NewParser().GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Request.PostData)
	assert.True(t, details.Request.PostData.Truncated)
	assert.Len(t, details.Request.PostData.TextPreview, maxBodyPreview+len(truncationMarker))
	assert.True(t, strings.HasSuffix(details.Request.PostData.TextPreview, truncationMarker))
	assert.Equal(t, int64(len(body)), details.Request.PostData.Size)
}

func TestGetRequestDetailsBinaryPostDataMetadataOnly(t *testing.T) {
	// Backspaces: a control byte that survives %q JSON escaping and fails the
	// text sniff (postData text is emitted as a plain JSON string, so raw
	// NUL bytes would render as invalid JSON escapes).
	parser := NewParser()
	body := "\b\b\bbinary-garbage"
	archive := createPostDataHAR(t, parser, nil, "application/octet-stream", body)

	details, err := parser.GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Request.PostData)
	require.NotEmpty(t, details.Request.PostData.Hash)
	assert.Empty(t, details.Request.PostData.TextPreview)
	assert.False(t, details.Request.PostData.Truncated)
	assert.Equal(t, int64(len(body)), details.Request.PostData.Size)
}

func TestLoadPolicyExcludedPostDataNotStored(t *testing.T) {
	parser := NewParser()
	policy := &LoadPolicy{ExcludeMimeTypes: []string{"multipart/"}}
	body := "some multipart payload"
	archive := createPostDataHAR(t, parser, policy, "multipart/form-data", body)

	details, err := parser.GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Request.PostData)
	assert.Empty(t, details.Request.PostData.Hash)

	sum := sha256.Sum256([]byte(body))
	ref := fmt.Sprintf("body:%x", sum)
	chunk, err := parser.GetResponseBody(ref, 0, 4096)
	assert.EqualError(t, err, "unknown body hash: "+ref)
	assert.Nil(t, chunk)
}

func TestGetRequestDetailsFormUrlencodedPostDataPreview(t *testing.T) {
	body := "a=1&b=2&name=har-mcp"
	archive := createPostDataHAR(t, NewParser(), nil, "application/x-www-form-urlencoded", body)

	details, err := NewParser().GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Request.PostData)
	assert.Equal(t, body, details.Request.PostData.TextPreview)
	assert.False(t, details.Request.PostData.Truncated)
}

func TestGetRequestDetailsInvalidID(t *testing.T) {
	harData := createTestHAR()
	parser := NewParser()
	archive := parseTestHAR(t, harData)

	// Test invalid format
	details, err := parser.GetRequestDetails(archive, "invalid_id")
	assert.Error(t, err)
	assert.Nil(t, details)
	assert.Contains(t, err.Error(), "invalid request ID format")

	// Test out of range
	details, err = parser.GetRequestDetails(archive, "request_999")
	assert.Error(t, err)
	assert.Nil(t, details)
	assert.Contains(t, err.Error(), "request ID out of range")
}

func TestGetRequestDetailsNilRequestReturnsError(t *testing.T) {
	harData := `{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [{
				"startedDateTime": "2023-01-01T00:00:00.000Z",
				"time": 100,
				"request": null,
				"response": null
			}]
		}
	}`
	archive := parseTestHAR(t, harData)

	// A legal-HAR entry with no request must not panic.
	details, err := NewParser().GetRequestDetails(archive, "request_0")

	assert.Error(t, err)
	assert.Nil(t, details)
	assert.Contains(t, err.Error(), "has no request data")
}

func TestRedactAuthHeaders(t *testing.T) {
	parser := NewParser()

	headers := []har.Header{
		{Name: "User-Agent", Value: "Mozilla/5.0"},
		{Name: "Authorization", Value: "Bearer secret-token"},
		{Name: "X-API-Key", Value: "api-key-123"},
		{Name: "Cookie", Value: "session=abc123"},
		{Name: "Content-Type", Value: "application/json"},
	}

	redacted := parser.redactAuthHeaders(headers)

	assert.Len(t, redacted, len(headers))

	// Check each header
	for _, header := range redacted {
		switch header.Name {
		case "User-Agent", "Content-Type":
			assert.NotEqual(t, "[REDACTED]", header.Value)
		case "Authorization", "X-API-Key", "Cookie":
			assert.Equal(t, "[REDACTED]", header.Value)
		}
	}
}

func TestGetRequestDetailsRedactsCookieValues(t *testing.T) {
	harData := `{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [{
				"startedDateTime": "2023-01-01T00:00:00.000Z",
				"time": 100,
				"request": {"method": "GET", "url": "https://example.com", "httpVersion": "HTTP/1.1",
					"cookies": [{"name": "session", "value": "secret-session"}, {"name": "JSESSIONID", "value": "sess-abc"}, {"name": "theme", "value": "dark"}],
					"headers": [], "queryString": [], "headersSize": 150, "bodySize": 0},
				"response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1",
					"cookies": [{"name": "auth_token", "value": "tok-secret"}, {"name": "PHPSESSID", "value": "php-sess"}, {"name": "lang", "value": "en"}],
					"headers": [], "content": {"size": 0, "mimeType": "text/plain"}, "redirectURL": "", "headersSize": 200, "bodySize": 0}
			}]
		}
	}`
	archive := parseTestHAR(t, harData)

	details, err := NewParser().GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	// Session-bearing cookies are redacted in both directions; ordinary
	// cookies keep their values.
	for _, c := range details.Request.Cookies {
		switch c.Name {
		case "session", "JSESSIONID":
			assert.Equal(t, "[REDACTED]", c.Value)
		case "theme":
			assert.Equal(t, "dark", c.Value)
		}
	}
	for _, c := range details.Response.Cookies {
		switch c.Name {
		case "auth_token", "PHPSESSID":
			assert.Equal(t, "[REDACTED]", c.Value)
		case "lang":
			assert.Equal(t, "en", c.Value)
		}
	}
}

// Test flexible parsing

func TestParseFlexibleTime(t *testing.T) {
	// HAR with float time values
	harData := `{
		"log": {
			"version": "1.2",
			"creator": {
				"name": "test-creator",
				"version": "1.0"
			},
			"entries": [
				{
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"time": 123.456,
					"request": {
						"method": "GET",
						"url": "https://example.com",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"queryString": [],
						"headersSize": 150,
						"bodySize": 0
					},
					"response": {
						"status": 200,
						"statusText": "OK",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"content": {
							"size": 1024,
							"mimeType": "text/html"
						},
						"redirectURL": "",
						"headersSize": 200,
						"bodySize": 1024
					},
					"timings": {
						"send": 1.5,
						"wait": 50.75,
						"receive": 71.206
					}
				}
			]
		}
	}`

	parser := NewParser()
	reader := strings.NewReader(harData)
	archive, err := parser.Parse(reader)

	require.NoError(t, err)
	require.NotNil(t, archive)
	assert.Len(t, archive.Log.Entries, 1)

	entry := archive.Log.Entries[0]
	assert.Equal(t, int64(123), entry.Time) // Should be rounded down from 123.456

	// Check timings
	assert.NotNil(t, entry.Timings)
	assert.Equal(t, int64(1), entry.Timings.Send)     // Rounded down from 1.5
	assert.Equal(t, int64(50), entry.Timings.Wait)    // Rounded down from 50.75
	assert.Equal(t, int64(71), entry.Timings.Receive) // Rounded down from 71.206
}

func TestParseTextContent(t *testing.T) {
	// HAR with plain text content (not base64)
	harData := `{
		"log": {
			"version": "1.2",
			"creator": {
				"name": "test-creator",
				"version": "1.0"
			},
			"entries": [
				{
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"time": 100,
					"request": {
						"method": "GET",
						"url": "https://example.com/api",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"queryString": [],
						"headersSize": 150,
						"bodySize": 0
					},
					"response": {
						"status": 200,
						"statusText": "OK",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"content": {
							"size": 13,
							"mimeType": "application/json",
							"text": "{\"ok\": true}"
						},
						"redirectURL": "",
						"headersSize": 200,
						"bodySize": 13
					}
				}
			]
		}
	}`

	parser := NewParser()
	reader := strings.NewReader(harData)
	archive, err := parser.Parse(reader)

	require.NoError(t, err)
	require.NotNil(t, archive)
	assert.Len(t, archive.Log.Entries, 1)

	entry := archive.Log.Entries[0]
	assert.NotNil(t, entry.Response)
	assert.NotNil(t, entry.Response.Content)
	assert.Equal(t, []byte(`{"ok": true}`), entry.Response.Content.Text)
}

func TestParseBase64Content(t *testing.T) {
	// HAR with base64 encoded content
	harData := `{
		"log": {
			"version": "1.2",
			"creator": {
				"name": "test-creator",
				"version": "1.0"
			},
			"entries": [
				{
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"time": 100,
					"request": {
						"method": "GET",
						"url": "https://example.com/image",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"queryString": [],
						"headersSize": 150,
						"bodySize": 0
					},
					"response": {
						"status": 200,
						"statusText": "OK",
						"httpVersion": "HTTP/1.1",
						"cookies": [],
						"headers": [],
						"content": {
							"size": 11,
							"mimeType": "image/png",
							"text": "SGVsbG8gV29ybGQ=",
							"encoding": "base64"
						},
						"redirectURL": "",
						"headersSize": 200,
						"bodySize": 11
					}
				}
			]
		}
	}`

	parser := NewParser()
	reader := strings.NewReader(harData)
	archive, err := parser.Parse(reader)

	require.NoError(t, err)
	require.NotNil(t, archive)
	assert.Len(t, archive.Log.Entries, 1)

	entry := archive.Log.Entries[0]
	assert.NotNil(t, entry.Response)
	assert.NotNil(t, entry.Response.Content)
	// Base64 decoded "SGVsbG8gV29ybGQ=" is "Hello World"
	assert.Equal(t, []byte("Hello World"), entry.Response.Content.Text)
}

func TestParseComplexHAR(t *testing.T) {
	// HAR with mixed content types and additional fields
	harData := `{
		"log": {
			"version": "1.2",
			"creator": {
				"name": "WebInspector",
				"version": "537.36"
			},
			"browser": {
				"name": "Chrome",
				"version": "120.0"
			},
			"pages": [
				{
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"id": "page_1",
					"title": "Test Page"
				}
			],
			"entries": [
				{
					"_id": "entry1",
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"time": 256.789,
					"request": {
						"method": "POST",
						"url": "https://api.example.com/data",
						"httpVersion": "HTTP/2.0",
						"cookies": [
							{"name": "session", "value": "abc123"}
						],
						"headers": [
							{"name": "Authorization", "value": "Bearer token123"},
							{"name": "Content-Type", "value": "application/json"}
						],
						"queryString": [],
						"postData": {
							"mimeType": "application/json",
							"text": "{\"data\": \"test\"}"
						},
						"headersSize": 250,
						"bodySize": 17
					},
					"response": {
						"status": 201,
						"statusText": "Created",
						"httpVersion": "HTTP/2.0",
						"cookies": [],
						"headers": [
							{"name": "Content-Type", "value": "application/json"}
						],
						"content": {
							"size": 29,
							"mimeType": "application/json",
							"text": "{\"id\": 123, \"status\": \"ok\"}"
						},
						"redirectURL": "",
						"headersSize": 150,
						"bodySize": 29
					},
					"cache": {},
					"timings": {
						"blocked": 0.5,
						"dns": 5.2,
						"connect": 15.7,
						"send": 0.402,
						"wait": 200.987,
						"receive": 34.0,
						"ssl": 10.5
					},
					"serverIPAddress": "93.184.216.34",
					"connection": "12345"
				}
			]
		}
	}`

	parser := NewParser()
	reader := strings.NewReader(harData)
	archive, err := parser.Parse(reader)

	require.NoError(t, err)
	require.NotNil(t, archive)
	assert.Len(t, archive.Log.Entries, 1)

	entry := archive.Log.Entries[0]
	assert.Equal(t, "entry1", entry.ID)
	assert.Equal(t, int64(256), entry.Time) // Rounded down from 256.789

	// Check request
	assert.Equal(t, "POST", entry.Request.Method)
	assert.Len(t, entry.Request.Cookies, 1)
	assert.Len(t, entry.Request.Headers, 2)

	// Check response
	assert.Equal(t, 201, entry.Response.Status)
	assert.Equal(t, []byte(`{"id": 123, "status": "ok"}`), entry.Response.Content.Text)

	// Check timings are converted properly
	assert.NotNil(t, entry.Timings)
	assert.Equal(t, int64(0), entry.Timings.Send)     // Rounded down from 0.402
	assert.Equal(t, int64(200), entry.Timings.Wait)   // Rounded down from 200.987
	assert.Equal(t, int64(34), entry.Timings.Receive) // Rounded down from 34.0

	// Check auth header is redacted when getting details
	details, err := parser.GetRequestDetails(archive, "request_0")
	require.NoError(t, err)

	var authHeader *har.Header
	for i := range details.Request.Headers {
		if details.Request.Headers[i].Name == "Authorization" {
			authHeader = &details.Request.Headers[i]
			break
		}
	}
	require.NotNil(t, authHeader)
	assert.Equal(t, "[REDACTED]", authHeader.Value)
}

// Body store tests

func TestBodyStoreDedupsIdenticalBodies(t *testing.T) {
	parser := NewParser()
	encoded := base64.StdEncoding.EncodeToString([]byte("same-response-body"))
	harData := fmt.Sprintf(`{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [
				{
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"time": 100,
					"request": {"method": "GET", "url": "https://example.com/a", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "queryString": [], "headersSize": 150, "bodySize": 0},
					"response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "content": {"size": 18, "mimeType": "application/json", "text": %q}, "redirectURL": "", "headersSize": 200, "bodySize": 18}
				},
				{
					"startedDateTime": "2023-01-01T00:00:01.000Z",
					"time": 100,
					"request": {"method": "GET", "url": "https://example.com/b", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "queryString": [], "headersSize": 150, "bodySize": 0},
					"response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "content": {"size": 18, "mimeType": "application/json", "text": %q}, "redirectURL": "", "headersSize": 200, "bodySize": 18}
				}
			]
		}
	}`, encoded, encoded)

	archive, err := parser.Parse(strings.NewReader(harData))
	require.NoError(t, err)
	require.Len(t, archive.Log.Entries, 2)

	first, err := parser.GetRequestDetails(archive, "request_0")
	require.NoError(t, err)
	second, err := parser.GetRequestDetails(archive, "request_1")
	require.NoError(t, err)

	require.NotEmpty(t, first.Response.Content.Hash)
	// Identical bodies share one hash reference...
	assert.Equal(t, first.Response.Content.Hash, second.Response.Content.Hash)
	// ...and are stored once, not once per entry.
	assert.Len(t, parser.bodies.entries, 1)
}

func TestGetRequestDetailsIncludesBodyHash(t *testing.T) {
	parser := NewParser()
	body := `{"ok": true}`
	archive := createResponseHARWithParser(t, parser, nil, "application/json", body)

	details, err := parser.GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Response.Content)
	sum := sha256.Sum256([]byte(body))
	assert.Equal(t, fmt.Sprintf("body:%x", sum), details.Response.Content.Hash)
}

func TestGetResponseBodyChunking(t *testing.T) {
	parser := NewParser()
	body := strings.Repeat("0123456789", 800) // 8000 bytes
	archive := createResponseHARWithParser(t, parser, nil, "text/plain", body)
	require.NotNil(t, archive)

	details, err := parser.GetRequestDetails(archive, "request_0")
	require.NoError(t, err)
	require.NotEmpty(t, details.Response.Content.Hash)
	ref := details.Response.Content.Hash

	// First chunk: bytes 0..4095, more remains.
	chunk, err := parser.GetResponseBody(ref, 0, 4096)
	require.NoError(t, err)
	assert.Equal(t, ref, chunk.Hash)
	assert.Equal(t, int64(len(body)), chunk.TotalSize)
	assert.Equal(t, "text/plain", chunk.MimeType)
	assert.Equal(t, body[:4096], chunk.Text)
	assert.True(t, chunk.Truncated)

	// Second chunk: the 3904-byte tail, nothing remains.
	chunk, err = parser.GetResponseBody(ref, 4096, 4096)
	require.NoError(t, err)
	assert.Equal(t, body[4096:], chunk.Text)
	assert.False(t, chunk.Truncated)
}

func TestGetResponseBodyClampsBounds(t *testing.T) {
	parser := NewParser()
	body := strings.Repeat("0123456789", 800) // 8000 bytes
	archive := createResponseHARWithParser(t, parser, nil, "text/plain", body)
	require.NotNil(t, archive)

	details, err := parser.GetRequestDetails(archive, "request_0")
	require.NoError(t, err)
	require.NotEmpty(t, details.Response.Content.Hash)
	ref := details.Response.Content.Hash

	// Negative offset clamps to 0; oversized limit caps at maxBodyChunk.
	chunk, err := parser.GetResponseBody(ref, -100, 1<<20)
	require.NoError(t, err)
	assert.Equal(t, 0, chunk.Offset)
	assert.Equal(t, maxBodyChunk, chunk.Limit)
	assert.Equal(t, body, chunk.Text)

	// Offset past the end yields an empty, untruncated chunk.
	chunk, err = parser.GetResponseBody(ref, len(body)+100, 4096)
	require.NoError(t, err)
	assert.Empty(t, chunk.Text)
	assert.False(t, chunk.Truncated)
}

func TestGetResponseBodyBinaryMetadataOnly(t *testing.T) {
	parser := NewParser()
	body := "\x00\x01\x02\x03binary-garbage" // NUL bytes: never looks like text
	archive := createResponseHARWithParser(t, parser, nil, "video/mp4", body)
	require.NotNil(t, archive)

	details, err := parser.GetRequestDetails(archive, "request_0")
	require.NoError(t, err)
	require.NotEmpty(t, details.Response.Content.Hash)

	chunk, err := parser.GetResponseBody(details.Response.Content.Hash, 0, 4096)
	require.NoError(t, err)
	assert.Equal(t, int64(len(body)), chunk.TotalSize)
	assert.Equal(t, "video/mp4", chunk.MimeType)
	assert.Empty(t, chunk.Text)
	assert.NotEmpty(t, chunk.Note)
	assert.False(t, chunk.Truncated)
}

func TestGetResponseBodyUnknownHash(t *testing.T) {
	chunk, err := NewParser().GetResponseBody("body:"+strings.Repeat("0", 64), 0, 4096)
	assert.Error(t, err)
	assert.Nil(t, chunk)
	assert.Contains(t, err.Error(), "unknown body hash")
}

// GetEntries tests

func TestGetEntriesKeepsQueryParams(t *testing.T) {
	harData := `{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [
				{
					"startedDateTime": "2023-01-01T00:00:00.000Z",
					"time": 100,
					"request": {"method": "GET", "url": "https://example.com/_next/static/chunks/4051.8ef8f2c36aab35cf.js?dpl=dpl_3Jw8vhcQQWEZCvVexEep", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "queryString": [], "headersSize": 150, "bodySize": 0},
					"response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "content": {"size": 11, "mimeType": "application/javascript"}, "redirectURL": "", "headersSize": 200, "bodySize": 11}
				},
				{
					"startedDateTime": "2023-01-01T00:00:01.000Z",
					"time": 100,
					"request": {"method": "GET", "url": "https://example.com/api/resource?id=1", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "queryString": [], "headersSize": 150, "bodySize": 0},
					"response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "content": {"size": 11, "mimeType": "application/json"}, "redirectURL": "", "headersSize": 200, "bodySize": 11}
				},
				{
					"startedDateTime": "2023-01-01T00:00:02.000Z",
					"time": 100,
					"request": {"method": "GET", "url": "https://example.com/api/resource?id=2", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "queryString": [], "headersSize": 150, "bodySize": 0},
					"response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "content": {"size": 11, "mimeType": "application/json"}, "redirectURL": "", "headersSize": 200, "bodySize": 11}
				}
			]
		}
	}`
	archive := parseTestHAR(t, harData)

	entries, total := NewParser().GetEntries(archive, "", "", 0, 10)

	assert.Equal(t, 3, total)
	require.Len(t, entries, 3)
	// Query params are kept so /api/resource?id=1 and ?id=2 stay
	// distinguishable in the index...
	assert.Contains(t, entries[1].URL, "id=1")
	assert.Contains(t, entries[2].URL, "id=2")
	// ...and the fragment-free URL under 100 chars is shown whole.
	assert.Equal(t, "https://example.com/_next/static/chunks/4051.8ef8f2c36aab35cf.js?dpl=dpl_3Jw8vhcQQWEZCvVexEep", entries[0].URL)
}

func TestGetEntriesFilterIsCaseInsensitiveSubstring(t *testing.T) {
	archive := parseTestHAR(t, createMultipleEntriesHAR())

	// Uppercase filter matches lowercase URLs.
	entries, total := NewParser().GetEntries(archive, "API/USERS", "", 0, 10)
	assert.Equal(t, 3, total)
	require.Len(t, entries, 3)

	// A filter containing query text matches nothing when the URL carries no
	// query string at all.
	entries, total = NewParser().GetEntries(archive, "users?x", "", 0, 10)
	assert.Equal(t, 0, total)
	assert.Empty(t, entries)
}

func TestGetEntriesPagination(t *testing.T) {
	archive := parseTestHAR(t, createMultipleEntriesHAR())

	entries, total := NewParser().GetEntries(archive, "", "", 0, 2)
	assert.Equal(t, 3, total)
	require.Len(t, entries, 2)
	assert.Equal(t, "request_0", entries[0].RequestID)
	assert.Equal(t, "request_1", entries[1].RequestID)

	// A page starting near the end is shorter than the limit.
	entries, total = NewParser().GetEntries(archive, "", "", 2, 2)
	assert.Equal(t, 3, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "request_2", entries[0].RequestID)
}

func TestGetEntriesIncludesBodyHash(t *testing.T) {
	parser := NewParser()
	body := `{"ok": true}`
	archive := createResponseHARWithParser(t, parser, nil, "application/json", body)

	entries, total := parser.GetEntries(archive, "", "", 0, 10)

	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	sum := sha256.Sum256([]byte(body))
	assert.Equal(t, fmt.Sprintf("body:%x", sum), entries[0].BodyHash)
	assert.Equal(t, 200, entries[0].Status)
	assert.Equal(t, "application/json", entries[0].MimeType)
	assert.Equal(t, int64(len(body)), entries[0].Size)
}

// Load policy tests

func TestLoadPolicyExcludedMimeNotStored(t *testing.T) {
	parser := NewParser()
	policy := &LoadPolicy{ExcludeMimeTypes: []string{"video/", "image/*"}}
	body := "fake mp4 bytes"
	archive := createResponseHARWithPolicy(t, parser, policy, nil, "video/mp4", body)

	details, err := parser.GetRequestDetails(archive, "request_0")
	require.NoError(t, err)
	require.NotNil(t, details.Response.Content)
	assert.Empty(t, details.Response.Content.Hash)

	sum := sha256.Sum256([]byte(body))
	ref := fmt.Sprintf("body:%x", sum)
	chunk, err := parser.GetResponseBody(ref, 0, 4096)
	assert.EqualError(t, err, "unknown body hash: "+ref)
	assert.Nil(t, chunk)
}

func TestLoadPolicyEmptyMimePatternStoresEverything(t *testing.T) {
	parser := NewParser()
	policy := &LoadPolicy{ExcludeMimeTypes: []string{"", "*"}}
	body := "should be stored"
	archive := createResponseHARWithPolicy(t, parser, policy, nil, "video/mp4", body)

	details, err := parser.GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Response.Content)
	// "" and "*" are ignored, not treated as match-everything exclusions.
	require.NotEmpty(t, details.Response.Content.Hash)
	chunk, err := parser.GetResponseBody(details.Response.Content.Hash, 0, 4096)
	require.NoError(t, err)
	assert.Equal(t, body, chunk.Text)
}

func TestLoadPolicyMaxKeepBytesNotStored(t *testing.T) {
	parser := NewParser()
	policy := &LoadPolicy{MaxKeepBytes: 100}

	big := strings.Repeat("x", 200)
	bigArchive := createResponseHARWithPolicy(t, parser, policy, nil, "text/plain", big)
	bigDetails, err := parser.GetRequestDetails(bigArchive, "request_0")
	require.NoError(t, err)
	require.NotNil(t, bigDetails.Response.Content)
	assert.Empty(t, bigDetails.Response.Content.Hash)
	bigSum := sha256.Sum256([]byte(big))
	bigRef := fmt.Sprintf("body:%x", bigSum)
	_, err = parser.GetResponseBody(bigRef, 0, 4096)
	assert.EqualError(t, err, "unknown body hash: "+bigRef)

	small := "small body"
	smallArchive := createResponseHARWithPolicy(t, parser, policy, nil, "text/plain", small)
	smallDetails, err := parser.GetRequestDetails(smallArchive, "request_0")
	require.NoError(t, err)
	require.NotNil(t, smallDetails.Response.Content)
	require.NotEmpty(t, smallDetails.Response.Content.Hash)
	chunk, err := parser.GetResponseBody(smallDetails.Response.Content.Hash, 0, 4096)
	require.NoError(t, err)
	assert.Equal(t, small, chunk.Text)
}

func TestParseReplacesBodyStore(t *testing.T) {
	parser := NewParser()

	first := createResponseHARWithParser(t, parser, nil, "text/plain", "first body")
	firstDetails, err := parser.GetRequestDetails(first, "request_0")
	require.NoError(t, err)
	require.NotNil(t, firstDetails.Response.Content)
	require.NotEmpty(t, firstDetails.Response.Content.Hash)

	second := createResponseHARWithParser(t, parser, nil, "text/plain", "second body")
	secondDetails, err := parser.GetRequestDetails(second, "request_0")
	require.NoError(t, err)
	require.NotNil(t, secondDetails.Response.Content)
	require.NotEmpty(t, secondDetails.Response.Content.Hash)
	assert.NotEqual(t, firstDetails.Response.Content.Hash, secondDetails.Response.Content.Hash)

	// The first load's body is gone after the re-load...
	_, err = parser.GetResponseBody(firstDetails.Response.Content.Hash, 0, 4096)
	assert.EqualError(t, err, "unknown body hash: "+firstDetails.Response.Content.Hash)

	// ...while the second load's body is fetchable.
	chunk, err := parser.GetResponseBody(secondDetails.Response.Content.Hash, 0, 4096)
	require.NoError(t, err)
	assert.Equal(t, "second body", chunk.Text)
}

// URL surface tests: query params are kept, redacted, and capped

func TestGetEntriesCapsLongURL(t *testing.T) {
	tail := "TAILMARKER"
	longValue := strings.Repeat("a", 100) + tail + strings.Repeat("b", 40)
	url := "https://example.com/api/search?q=" + longValue
	harData := fmt.Sprintf(`{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [{
				"startedDateTime": "2023-01-01T00:00:00.000Z",
				"time": 100,
				"request": {"method": "GET", "url": %q, "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "queryString": [], "headersSize": 150, "bodySize": 0},
				"response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "content": {"size": 2, "mimeType": "application/json"}, "redirectURL": "", "headersSize": 200, "bodySize": 2}
			}]
		}
	}`, url)
	archive := parseTestHAR(t, harData)

	// The display is capped at maxIndexURLLen with an ellipsis...
	entries, total := NewParser().GetEntries(archive, "", "", 0, 10)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.True(t, strings.HasSuffix(entries[0].URL, "…"))
	assert.LessOrEqual(t, len(entries[0].URL), maxIndexURLLen+len("…"))
	assert.NotContains(t, entries[0].URL, tail)

	// ...but the filter matches the FULL URL, including the part past the cap.
	entries, total = NewParser().GetEntries(archive, tail, "", 0, 10)
	assert.Equal(t, 1, total)
	assert.Equal(t, "request_0", entries[0].RequestID)
}

func TestGetEntriesRedactsSensitiveQueryValues(t *testing.T) {
	harData := fmt.Sprintf(`{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [{
				"startedDateTime": "2023-01-01T00:00:00.000Z",
				"time": 100,
				"request": {"method": "GET", "url": %q, "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "queryString": [], "headersSize": 150, "bodySize": 0},
				"response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "content": {"size": 2, "mimeType": "application/json"}, "redirectURL": "", "headersSize": 200, "bodySize": 2}
			}]
		}
	}`, "https://example.com/api/data?token=supersecret&id=42")
	archive := parseTestHAR(t, harData)

	entries, total := NewParser().GetEntries(archive, "", "", 0, 10)

	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].URL, "token=[REDACTED]")
	assert.NotContains(t, entries[0].URL, "supersecret")
	assert.Contains(t, entries[0].URL, "id=42")
}

func TestGetRequestDetailsRedactsQueryValues(t *testing.T) {
	harData := fmt.Sprintf(`{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [{
				"startedDateTime": "2023-01-01T00:00:00.000Z",
				"time": 100,
				"request": {"method": "GET", "url": %q, "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "queryString": [{"name": "token", "value": "supersecret"}, {"name": "id", "value": "42"}], "headersSize": 150, "bodySize": 0},
				"response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "content": {"size": 2, "mimeType": "application/json"}, "redirectURL": "", "headersSize": 200, "bodySize": 2}
			}]
		}
	}`, "https://example.com/api/data?token=supersecret&id=42")
	archive := parseTestHAR(t, harData)

	details, err := NewParser().GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	assert.Contains(t, details.Request.URL, "token=[REDACTED]")
	assert.NotContains(t, details.Request.URL, "supersecret")
	for _, q := range details.Request.QueryString {
		switch q.Name {
		case "token":
			assert.Equal(t, "[REDACTED]", q.Value)
		case "id":
			assert.Equal(t, "42", q.Value)
		}
	}
}

func TestGetRequestIDsForURLMethodMatchesRedactedURL(t *testing.T) {
	harData := fmt.Sprintf(`{
		"log": {
			"version": "1.2",
			"creator": {"name": "test-creator", "version": "1.0"},
			"entries": [{
				"startedDateTime": "2023-01-01T00:00:00.000Z",
				"time": 100,
				"request": {"method": "GET", "url": %q, "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "queryString": [], "headersSize": 150, "bodySize": 0},
				"response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1", "cookies": [], "headers": [], "content": {"size": 2, "mimeType": "application/json"}, "redirectURL": "", "headersSize": 200, "bodySize": 2}
			}]
		}
	}`, "https://example.com/api/data?token=abc123")
	archive := parseTestHAR(t, harData)
	parser := NewParser()

	// The redacted form (what an agent copies from the index) matches...
	ids := parser.GetRequestIDsForURLMethod(archive, "https://example.com/api/data?token=[REDACTED]", "GET")
	assert.Equal(t, []string{"request_0"}, ids)
	// ...and so does the raw form with any token value: matching runs on the
	// normalized, redacted URL, so secrets never need to be re-typed.
	ids = parser.GetRequestIDsForURLMethod(archive, "https://example.com/api/data?token=other", "GET")
	assert.Equal(t, []string{"request_0"}, ids)
}

// Body store reference format tests

func TestBodyHashIsFullSHA256(t *testing.T) {
	parser := NewParser()
	body := `{"ok": true}`
	archive := createResponseHARWithParser(t, parser, nil, "application/json", body)

	details, err := parser.GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Response.Content)
	assert.Regexp(t, `^body:[0-9a-f]{64}$`, details.Response.Content.Hash)
}

// Mime sniffing and preview truncation tests

func TestMislabeledJSONGetsPreviewAndFetch(t *testing.T) {
	parser := NewParser()
	body := `{"ok": true}`
	archive := createResponseHARWithParser(t, parser, nil, "application/octet-stream", body)

	details, err := parser.GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Response.Content)
	// The mime says binary, but the bytes are JSON: sniffed as readable.
	assert.Equal(t, body, details.Response.Content.TextPreview)
	assert.False(t, details.Response.Content.Truncated)
	require.NotEmpty(t, details.Response.Content.Hash)

	chunk, err := parser.GetResponseBody(details.Response.Content.Hash, 0, 4096)
	require.NoError(t, err)
	assert.Equal(t, body, chunk.Text)
	assert.Empty(t, chunk.Note)
}

func TestBinaryBytesGetNoPreview(t *testing.T) {
	parser := NewParser()
	archive := createResponseHARWithParser(t, parser, nil, "application/octet-stream", "\x00\x01\x02\x03")

	details, err := parser.GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Response.Content)
	assert.Empty(t, details.Response.Content.TextPreview)
	assert.False(t, details.Response.Content.Truncated)

	chunk, err := parser.GetResponseBody(details.Response.Content.Hash, 0, 4096)
	require.NoError(t, err)
	assert.Empty(t, chunk.Text)
	assert.NotEmpty(t, chunk.Note)
}

func TestPreviewCutAtRuneBoundary(t *testing.T) {
	body := strings.Repeat("a", maxBodyPreview-1) + "é" + strings.Repeat("b", 50)
	archive := createResponseHAR(t, nil, "text/plain", body)

	details, err := NewParser().GetRequestDetails(archive, "request_0")

	require.NoError(t, err)
	require.NotNil(t, details.Response.Content)
	assert.True(t, details.Response.Content.Truncated)
	assert.True(t, strings.HasSuffix(details.Response.Content.TextPreview, truncationMarker))
	// The body portion of the cut never exceeds the cap and never splits a
	// rune: the preview minus its marker stays valid UTF-8.
	bodyPart := strings.TrimSuffix(details.Response.Content.TextPreview, truncationMarker)
	assert.LessOrEqual(t, len(bodyPart), maxBodyPreview)
	assert.True(t, utf8.ValidString(bodyPart))
}

// BenchmarkParseWithBodies measures parse-time cost and allocation for a HAR
// carrying many sizable bodies — the memory/GC profile the body store is
// responsible for.
func BenchmarkParseWithBodies(b *testing.B) {
	body := strings.Repeat("payload-data-", 4096) // ~49KB
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	var sb strings.Builder
	sb.WriteString(`{"log":{"version":"1.2","creator":{"name":"bench","version":"1"},"entries":[`)
	for i := 0; i < 50; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"startedDateTime":"2023-01-01T00:00:00Z","time":1,"request":{"method":"GET","url":"https://example.com/%d","httpVersion":"HTTP/1.1","cookies":[],"headers":[],"queryString":[],"headersSize":0,"bodySize":0},"response":{"status":200,"statusText":"OK","httpVersion":"HTTP/1.1","cookies":[],"headers":[],"content":{"size":%d,"mimeType":"text/plain","text":%q},"redirectURL":"","headersSize":0,"bodySize":%d}}`, i, len(body), encoded, len(body))
	}
	sb.WriteString(`]}}`)
	data := sb.String()

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := NewParser().Parse(strings.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}

// FuzzBodyReadable checks the text/binary classifier on arbitrary mime types
// and bytes: it must never panic, and the verdict must not depend on whether
// the whole body or only its leading sample is inspected (the sniff only
// ever looks at the first textSniffSample bytes).
func FuzzBodyReadable(f *testing.F) {
	for _, seed := range []struct {
		mime    string
		content []byte
	}{
		{"application/json", []byte(`{"ok": true}`)},
		{"text/html", []byte("<html>ok</html>")},
		{"application/octet-stream", []byte(`{"mislabeled": 1}`)},
		{"application/octet-stream", []byte("\x00\x01\x02binary")},
		{"video/mp4", []byte("fakemp4bytes")},
		{"", []byte("no mime at all")},
	} {
		f.Add(seed.mime, seed.content)
	}

	f.Fuzz(func(t *testing.T, mime string, content []byte) {
		sample := content
		if len(sample) > textSniffSample {
			sample = sample[:textSniffSample]
		}
		if bodyReadable(mime, content) != bodyReadable(mime, sample) {
			t.Fatalf("bodyReadable(%q, %d bytes) disagrees with its %d-byte sample", mime, len(content), len(sample))
		}
	})
}
