package har

import (
	"strings"
	"testing"

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
