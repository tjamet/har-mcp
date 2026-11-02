package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// examplePath is the HAR shipped at the repository root: 3 entries, request_0
// is a GET /users carrying an Authorization header.
func examplePath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "example.har"))
	require.NoError(t, err)
	return p
}

func loadedServer(t *testing.T) *HARServer {
	t.Helper()
	h := NewHARServer()
	require.NoError(t, h.loadHAR(examplePath(t)))
	return h
}

func callRequest(args any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, res)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(mcp.TextContent)
	require.True(t, ok, "content is %T", res.Content[0])
	return text.Text
}

func TestCreateToolsNamesAndRequiredArguments(t *testing.T) {
	want := map[string][]string{
		"load_har":            {"source"},
		"list_urls_methods":   nil,
		"get_request_ids":     {"url", "method"},
		"get_request_details": {"request_id"},
	}

	tools := NewHARServer().createTools()

	got := map[string][]string{}
	for _, st := range tools {
		assert.Equal(t, "object", st.Tool.InputSchema.Type, st.Tool.Name)
		assert.NotEmpty(t, st.Tool.Description, st.Tool.Name)
		assert.NotNil(t, st.Handler, st.Tool.Name)
		for _, r := range st.Tool.InputSchema.Required {
			assert.Contains(t, st.Tool.InputSchema.Properties, r, st.Tool.Name)
		}
		got[st.Tool.Name] = st.Tool.InputSchema.Required
	}
	assert.Equal(t, want, got)
}

func TestLoadHARRejectsBadSource(t *testing.T) {
	h := NewHARServer()

	err := h.loadHAR(filepath.Join(t.TempDir(), "missing.har"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load HAR")
	assert.Nil(t, h.harData)
}

func TestLoadHARHandler(t *testing.T) {
	t.Run("loads the example", func(t *testing.T) {
		h := NewHARServer()

		res, err := h.handleLoadHAR(context.Background(), callRequest(map[string]any{"source": examplePath(t)}))

		require.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Equal(t, "Successfully loaded HAR file with 3 entries", resultText(t, res))
	})

	t.Run("bad source is a tool error and keeps the previous HAR", func(t *testing.T) {
		h := loadedServer(t)

		res, err := h.handleLoadHAR(context.Background(), callRequest(map[string]any{"source": "/nonexistent/x.har"}))

		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, resultText(t, res), "Error loading HAR file")
		assert.NotNil(t, h.harData)
	})

	t.Run("malformed arguments", func(t *testing.T) {
		res, err := NewHARServer().handleLoadHAR(context.Background(), callRequest("not an object"))

		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, resultText(t, res), "Invalid arguments")
	})
}

func TestHandlersRequireLoadedHAR(t *testing.T) {
	h := NewHARServer()
	handlers := map[string]server.ToolHandlerFunc{
		"list_urls_methods":   h.handleListURLsMethods,
		"get_request_ids":     h.handleGetRequestIDs,
		"get_request_details": h.handleGetRequestDetails,
	}
	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			res, err := handler(context.Background(), callRequest(map[string]any{}))

			require.NoError(t, err)
			assert.True(t, res.IsError)
			assert.Contains(t, resultText(t, res), "No HAR file loaded")
		})
	}
}

func TestListURLsMethods(t *testing.T) {
	res, err := loadedServer(t).handleListURLsMethods(context.Background(), callRequest(nil))

	require.NoError(t, err)
	assert.False(t, res.IsError)
	var entries []struct {
		URL        string   `json:"url"`
		Method     string   `json:"method"`
		RequestIDs []string `json:"request_ids"`
	}
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &entries))
	got := map[string][]string{}
	for _, e := range entries {
		got[e.Method+" "+e.URL] = e.RequestIDs
	}
	assert.Equal(t, map[string][]string{
		"GET https://api.example.com/users":    {"request_0"},
		"POST https://api.example.com/users":   {"request_1"},
		"GET https://api.example.com/users/42": {"request_2"},
	}, got)
}

func TestGetRequestIDs(t *testing.T) {
	h := loadedServer(t)

	t.Run("match", func(t *testing.T) {
		res, err := h.handleGetRequestIDs(context.Background(), callRequest(map[string]any{
			"url": "https://api.example.com/users", "method": "POST",
		}))

		require.NoError(t, err)
		assert.False(t, res.IsError)
		var ids []string
		require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &ids))
		assert.Equal(t, []string{"request_1"}, ids)
	})

	t.Run("no match is an empty result, not an error", func(t *testing.T) {
		res, err := h.handleGetRequestIDs(context.Background(), callRequest(map[string]any{
			"url": "https://nowhere.example", "method": "GET",
		}))

		require.NoError(t, err)
		assert.False(t, res.IsError)
		var ids []string
		require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &ids))
		assert.Empty(t, ids)
	})

	t.Run("malformed arguments", func(t *testing.T) {
		res, err := h.handleGetRequestIDs(context.Background(), callRequest("nope"))

		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, resultText(t, res), "Invalid arguments")
	})
}

func TestGetRequestDetails(t *testing.T) {
	h := loadedServer(t)

	t.Run("redacts authentication headers", func(t *testing.T) {
		res, err := h.handleGetRequestDetails(context.Background(), callRequest(map[string]any{"request_id": "request_0"}))

		require.NoError(t, err)
		assert.False(t, res.IsError)
		text := resultText(t, res)
		assert.NotContains(t, text, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
		var details struct {
			RequestID string `json:"request_id"`
			Request   struct {
				Method  string `json:"method"`
				URL     string `json:"url"`
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"request"`
		}
		require.NoError(t, json.Unmarshal([]byte(text), &details))
		assert.Equal(t, "request_0", details.RequestID)
		assert.Equal(t, "GET", details.Request.Method)
		assert.Equal(t, "https://api.example.com/users", details.Request.URL)
		headers := map[string]string{}
		for _, hd := range details.Request.Headers {
			headers[hd.Name] = hd.Value
		}
		assert.Equal(t, "[REDACTED]", headers["Authorization"])
		assert.Equal(t, "application/json", headers["Accept"])
	})

	t.Run("redacts every credential-bearing header, whatever its case", func(t *testing.T) {
		secrets := map[string]string{
			"Authorization":       "Bearer s3cret-authorization",
			"X-API-Key":           "s3cret-api-key",
			"x-auth-token":        "s3cret-auth-token",
			"COOKIE":              "session=s3cret-cookie",
			"Set-Cookie":          "session=s3cret-set-cookie",
			"Proxy-Authorization": "Basic s3cret-proxy",
		}
		headers := []map[string]string{{"name": "Accept", "value": "application/json"}}
		for name, value := range secrets {
			headers = append(headers, map[string]string{"name": name, "value": value})
		}
		doc, err := json.Marshal(map[string]any{"log": map[string]any{
			"version": "1.2",
			"creator": map[string]string{"name": "test", "version": "0"},
			"entries": []map[string]any{{
				"startedDateTime": "2024-01-15T10:30:00.000Z",
				"time":            1,
				"request": map[string]any{
					"method": "GET", "url": "https://api.example.com/x", "httpVersion": "HTTP/1.1",
					"cookies": []any{}, "headers": headers, "queryString": []any{},
					"headersSize": -1, "bodySize": 0,
				},
				"response": map[string]any{
					"status": 200, "statusText": "OK", "httpVersion": "HTTP/1.1",
					"cookies": []any{}, "headers": []any{},
					"content":     map[string]any{"size": 0, "mimeType": "text/plain"},
					"redirectURL": "", "headersSize": -1, "bodySize": 0,
				},
				"cache":   map[string]any{},
				"timings": map[string]any{"send": 0, "wait": 1, "receive": 0},
			}},
		}})
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "secrets.har")
		require.NoError(t, os.WriteFile(path, doc, 0o600))
		hs := NewHARServer()
		require.NoError(t, hs.loadHAR(path))

		res, err := hs.handleGetRequestDetails(context.Background(), callRequest(map[string]any{"request_id": "request_0"}))

		require.NoError(t, err)
		assert.False(t, res.IsError)
		text := resultText(t, res)
		for name, value := range secrets {
			assert.NotContains(t, text, value, name)
		}
		var details struct {
			Request struct {
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"request"`
		}
		require.NoError(t, json.Unmarshal([]byte(text), &details))
		got := map[string]string{}
		for _, hd := range details.Request.Headers {
			got[hd.Name] = hd.Value
		}
		for name := range secrets {
			assert.Equal(t, "[REDACTED]", got[name], name)
		}
		assert.Equal(t, "application/json", got["Accept"])
	})

	t.Run("unknown request id", func(t *testing.T) {
		res, err := h.handleGetRequestDetails(context.Background(), callRequest(map[string]any{"request_id": "request_99"}))

		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, resultText(t, res), "Error getting request details")
	})

	t.Run("malformed arguments", func(t *testing.T) {
		res, err := h.handleGetRequestDetails(context.Background(), callRequest("nope"))

		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, resultText(t, res), "Invalid arguments")
	})
}

// responseTimeout bounds each wait for the server so a silent server fails the
// test quickly instead of hanging to the global timeout.
const responseTimeout = 10 * time.Second

// session drives the real mcp-go stdio server, as main does, over in-memory pipes.
type session struct {
	t      *testing.T
	in     io.Writer
	out    *bufio.Scanner
	close  func()
	nextID int
}

func startSession(t *testing.T) *session {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- server.NewStdioServer(newMCPServer(NewHARServer())).Listen(ctx, inR, outW)
		outW.Close()
	}()
	closePipes := func() {
		cancel()
		inW.Close()
		outR.Close()
	}
	t.Cleanup(func() {
		closePipes()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("stdio server did not stop")
		}
	})
	return &session{t: t, in: inW, out: bufio.NewScanner(outR), close: closePipes}
}

func (s *session) send(v any) {
	s.t.Helper()
	line, err := json.Marshal(v)
	require.NoError(s.t, err)
	_, err = s.in.Write(append(line, '\n'))
	require.NoError(s.t, err)
}

// call sends a request and returns the "result" of its response.
func (s *session) call(method string, params any) json.RawMessage {
	s.t.Helper()
	s.nextID++
	s.send(map[string]any{"jsonrpc": "2.0", "id": s.nextID, "method": method, "params": params})
	scanned := make(chan bool, 1)
	go func() { scanned <- s.out.Scan() }()
	select {
	case ok := <-scanned:
		require.True(s.t, ok, "no response to %s: %v", method, s.out.Err())
	case <-time.After(responseTimeout):
		s.close() // unblocks the pending Scan and the server
		s.t.Fatalf("no response to %s within %s", method, responseTimeout)
	}
	var resp struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(s.t, json.Unmarshal(s.out.Bytes(), &resp))
	require.Nil(s.t, resp.Error, "%s failed: %s", method, s.out.Text())
	require.Equal(s.t, s.nextID, resp.ID)
	return resp.Result
}

func TestEndToEndOverStdio(t *testing.T) {
	s := startSession(t)

	var init struct {
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
		Capabilities struct {
			Tools *json.RawMessage `json:"tools"`
		} `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(s.call("initialize", map[string]any{
		"protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	}), &init))
	assert.Equal(t, "har-mcp", init.ServerInfo.Name)
	assert.Equal(t, "1.0.0", init.ServerInfo.Version)
	assert.NotNil(t, init.Capabilities.Tools)
	s.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})

	var list struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Type     string   `json:"type"`
				Required []string `json:"required"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(s.call("tools/list", map[string]any{}), &list))
	required := map[string][]string{}
	for _, tl := range list.Tools {
		assert.Equal(t, "object", tl.InputSchema.Type)
		required[tl.Name] = tl.InputSchema.Required
	}
	assert.Equal(t, map[string][]string{
		"load_har":            {"source"},
		"list_urls_methods":   nil,
		"get_request_ids":     {"url", "method"},
		"get_request_details": {"request_id"},
	}, required)

	callTool := func(name string, args map[string]any) (bool, string) {
		var res struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		require.NoError(t, json.Unmarshal(s.call("tools/call", map[string]any{"name": name, "arguments": args}), &res))
		require.Len(t, res.Content, 1)
		return res.IsError, res.Content[0].Text
	}

	isErr, text := callTool("list_urls_methods", map[string]any{})
	assert.True(t, isErr)
	assert.Contains(t, text, "No HAR file loaded")

	isErr, text = callTool("load_har", map[string]any{"source": examplePath(t)})
	assert.False(t, isErr)
	assert.Equal(t, "Successfully loaded HAR file with 3 entries", text)

	isErr, text = callTool("get_request_details", map[string]any{"request_id": "request_0"})
	assert.False(t, isErr)
	assert.Contains(t, text, "[REDACTED]")
	assert.NotContains(t, text, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
}
