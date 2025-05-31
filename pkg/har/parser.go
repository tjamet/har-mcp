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

	"github.com/google/martian/har"
)

// Parser handles HAR file parsing from various sources
type Parser struct{}

// NewParser creates a new HAR parser
func NewParser() *Parser {
	return &Parser{}
}

// ParseFromFile parses a HAR file from disk
func (p *Parser) ParseFromFile(path string) (*har.HAR, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open HAR file: %w", err)
	}
	defer file.Close() //nolint:errcheck

	return p.Parse(file)
}

// ParseFromURL parses a HAR file from an HTTP URL
func (p *Parser) ParseFromURL(harURL string) (*har.HAR, error) {
	resp, err := http.Get(harURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch HAR from URL: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch HAR: HTTP %d", resp.StatusCode)
	}

	return p.Parse(resp.Body)
}

// Parse parses a HAR file from the given reader
func (p *Parser) Parse(r io.Reader) (*har.HAR, error) {
	// Read all data so we can try multiple parsing approaches
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read HAR data: %w", err)
	}

	// First try standard parsing
	var harData har.HAR
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&harData); err == nil {
		// Standard parsing succeeded
		return &harData, nil
	}

	// If standard parsing failed, try flexible parsing
	var flexibleHAR FlexibleHAR
	decoder = json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&flexibleHAR); err != nil {
		return nil, fmt.Errorf("failed to parse HAR file: %w", err)
	}

	// Convert flexible HAR to standard HAR
	return flexibleHAR.ToStandardHAR(), nil
}

// URLMethodEntry represents a URL and method combination with associated request IDs
type URLMethodEntry struct {
	URL        string   `json:"url"`
	Method     string   `json:"method"`
	RequestIDs []string `json:"request_ids"`
}

// GetURLsAndMethods returns all unique URL and method combinations from the HAR
func (p *Parser) GetURLsAndMethods(harData *har.HAR) []URLMethodEntry {
	// Map to store unique URL+Method combinations and their request IDs
	urlMethodMap := make(map[string]*URLMethodEntry)

	for i, entry := range harData.Log.Entries {
		if entry.Request == nil {
			continue
		}

		key := fmt.Sprintf("%s|%s", entry.Request.URL, entry.Request.Method)
		requestID := fmt.Sprintf("request_%d", i)

		if existing, ok := urlMethodMap[key]; ok {
			existing.RequestIDs = append(existing.RequestIDs, requestID)
		} else {
			urlMethodMap[key] = &URLMethodEntry{
				URL:        entry.Request.URL,
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

		if entry.Request.URL == targetURL && entry.Request.Method == method {
			requestID := fmt.Sprintf("request_%d", i)
			requestIDs = append(requestIDs, requestID)
		}
	}

	return requestIDs
}

// RequestDetails represents the full details of a request with auth headers redacted
type RequestDetails struct {
	RequestID       string        `json:"request_id"`
	StartedDateTime string        `json:"started_datetime"`
	Time            float64       `json:"time"`
	Request         *RequestInfo  `json:"request"`
	Response        *har.Response `json:"response"`
	Cache           *har.Cache    `json:"cache,omitempty"`
	Timings         *har.Timings  `json:"timings,omitempty"`
	ServerIPAddress string        `json:"serverIPAddress,omitempty"`
	Connection      string        `json:"connection,omitempty"`
	Comment         string        `json:"comment,omitempty"`
}

// RequestInfo is like har.Request but with redacted auth headers
type RequestInfo struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	HTTPVersion string            `json:"httpVersion"`
	Cookies     []har.Cookie      `json:"cookies"`
	Headers     []har.Header      `json:"headers"`
	QueryString []har.QueryString `json:"queryString"`
	PostData    *har.PostData     `json:"postData,omitempty"`
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

	// Create request info with redacted headers
	requestInfo := &RequestInfo{
		Method:      entry.Request.Method,
		URL:         entry.Request.URL,
		HTTPVersion: entry.Request.HTTPVersion,
		Cookies:     entry.Request.Cookies,
		Headers:     p.redactAuthHeaders(entry.Request.Headers),
		QueryString: entry.Request.QueryString,
		PostData:    entry.Request.PostData,
		HeadersSize: entry.Request.HeadersSize,
		BodySize:    entry.Request.BodySize,
	}

	details := &RequestDetails{
		RequestID:       requestID,
		StartedDateTime: entry.StartedDateTime.Format(time.RFC3339),
		Time:            float64(entry.Time),
		Request:         requestInfo,
		Response:        entry.Response,
		Cache:           entry.Cache,
		Timings:         entry.Timings,
	}

	return details, nil
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
	// Check if it's a URL
	if u, err := url.Parse(source); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return p.ParseFromURL(source)
	}

	// Otherwise treat as file path
	return p.ParseFromFile(source)
}
