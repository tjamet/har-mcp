package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/google/martian/har"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	harParser "github.com/tjamet/har-mcp/pkg/har"
)

// HARServer implements the MCP server for HAR file analysis
type HARServer struct {
	parser  *harParser.Parser
	harData *har.HAR
}

// maxRequestIDs bounds get_request_ids results for the same reason.
const maxRequestIDs = 2000

// NewHARServer creates a new HAR MCP server
func NewHARServer() *HARServer {
	return &HARServer{
		parser: harParser.NewParser(),
	}
}

// loadHAR loads a HAR file from the given source, optionally applying a
// load policy that keeps excluded bodies out of the body store.
func (h *HARServer) loadHAR(source string, policy *harParser.LoadPolicy) error {
	harData, err := h.parser.ParseSourceWithPolicy(source, policy)
	if err != nil {
		return fmt.Errorf("failed to load HAR: %w", err)
	}
	h.harData = harData
	return nil
}

// createTools creates the server tools with their handlers
func (h *HARServer) createTools() []server.ServerTool {
	return []server.ServerTool{
		{
			Tool: mcp.Tool{
				Name:        "load_har",
				Description: "Load a HAR file from a file path or HTTP URL. The optional policy keeps matching bodies out of the body store: excluded bodies still appear in details but get no hash and cannot be fetched with get_response_body.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"source": map[string]interface{}{
							"type":        "string",
							"description": "File path or HTTP URL to the HAR file",
						},
						"policy": map[string]interface{}{
							"type":        "object",
							"description": "Optional load policy: bodies matching an excluded mime prefix or larger than maxKeepBytes are not stored",
							"properties": map[string]interface{}{
								"excludeMimeTypes": map[string]interface{}{
									"type":        "array",
									"items":       map[string]interface{}{"type": "string"},
									"description": "Mime type prefixes to exclude, case-insensitive (e.g. \"video/\", \"image/*\")",
								},
								"maxKeepBytes": map[string]interface{}{
									"type":        "number",
									"description": "Bodies larger than this many bytes are not stored (absent or <= 0: no limit)",
								},
							},
						},
					},
					Required: []string{"source"},
				},
			},
			Handler: h.handleLoadHAR,
		},
		{
			Tool: mcp.Tool{
				Name:        "list_entries",
				Description: "List all HAR entries as one compact row each — method, status, mime type, size, timing, and body hash. URLs keep query params (capped at 100 chars; sensitive values redacted). This is the primary index: call this first, then get_request_details, then get_response_body.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"filter": map[string]interface{}{
							"type":        "string",
							"description": "Substring match on the request URL including query params (sensitive values matched in redacted form)",
						},
						"method": map[string]interface{}{
							"type":        "string",
							"description": "The HTTP method to filter by (GET, POST, etc.)",
						},
						"offset": map[string]interface{}{
							"type":        "number",
							"description": "Row offset into the matching entries (default 0)",
						},
						"limit": map[string]interface{}{
							"type":        "number",
							"description": "Maximum number of rows to return (default 200, max 1000)",
						},
					},
				},
			},
			Handler: h.handleListEntries,
		},
		{
			Tool: mcp.Tool{
				Name:        "get_request_ids",
				Description: "Get all request IDs for a specific URL and HTTP method",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "The URL to filter by (matched in normalized form: fragment stripped, sensitive query values redacted)",
						},
						"method": map[string]interface{}{
							"type":        "string",
							"description": "The HTTP method to filter by (GET, POST, etc.)",
						},
					},
					Required: []string{"url", "method"},
				},
			},
			Handler: h.handleGetRequestIDs,
		},
		{
			Tool: mcp.Tool{
				Name:        "get_request_details",
				Description: "Get full request details by request ID (authentication headers and sensitive query values redacted). Text bodies up to 4KB are fully inlined in response.content.textPreview; larger bodies return the first 4KB (ending with a [TRUNCATED] marker) plus a content.hash for get_response_body.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"request_id": map[string]interface{}{
							"type":        "string",
							"description": "The request ID to retrieve details for",
						},
					},
					Required: []string{"request_id"},
				},
			},
			Handler: h.handleGetRequestDetails,
		},
		{
			Tool: mcp.Tool{
				Name:        "get_response_body",
				Description: "Fetch a chunk of a stored response body by content hash (returned as response.content.hash by get_request_details). Text bodies return the decoded bytes between offset and offset+limit; binary bodies return metadata only.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"hash": map[string]interface{}{
							"type":        "string",
							"description": "Content hash reference of the body to fetch",
						},
						"offset": map[string]interface{}{
							"type":        "number",
							"description": "Byte offset into the decoded body (default 0)",
						},
						"limit": map[string]interface{}{
							"type":        "number",
							"description": "Maximum number of bytes to return (default 4096, max 65536)",
						},
					},
					Required: []string{"hash"},
				},
			},
			Handler: h.handleGetResponseBody,
		},
	}
}

// handleLoadHAR handles the load_har tool call
func (h *HARServer) handleLoadHAR(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Source string                `json:"source"`
		Policy *harParser.LoadPolicy `json:"policy"`
	}
	if err := request.BindArguments(&args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	if err := h.loadHAR(args.Source, args.Policy); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading HAR file: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully loaded HAR file with %d entries", len(h.harData.Log.Entries))), nil
}

// handleListEntries handles the list_entries tool call
func (h *HARServer) handleListEntries(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.harData == nil {
		return mcp.NewToolResultError("No HAR file loaded. Please load a HAR file first using load_har."), nil
	}

	var args struct {
		Filter string `json:"filter"`
		Method string `json:"method"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := request.BindArguments(&args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}
	// Clamp here too so the echoed offset/limit and truncated math match the
	// page GetEntries actually returned.
	if args.Offset < 0 {
		args.Offset = 0
	}
	if args.Limit <= 0 {
		args.Limit = 200
	}
	if args.Limit > 1000 {
		args.Limit = 1000
	}

	entries, total := h.parser.GetEntries(h.harData, args.Filter, args.Method, args.Offset, args.Limit)
	data, err := json.MarshalIndent(struct {
		Entries   []harParser.EntrySummary `json:"entries"`
		Total     int                      `json:"total"`
		Offset    int                      `json:"offset"`
		Limit     int                      `json:"limit"`
		Truncated bool                     `json:"truncated"`
	}{Entries: entries, Total: total, Offset: args.Offset, Limit: args.Limit, Truncated: args.Offset+len(entries) < total}, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal entries: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

// handleGetRequestIDs handles the get_request_ids tool call
func (h *HARServer) handleGetRequestIDs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.harData == nil {
		return mcp.NewToolResultError("No HAR file loaded. Please load a HAR file first using load_har."), nil
	}

	var args struct {
		URL    string `json:"url"`
		Method string `json:"method"`
	}
	if err := request.BindArguments(&args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	requestIDs := h.parser.GetRequestIDsForURLMethod(h.harData, args.URL, args.Method)
	total := len(requestIDs)
	if total > maxRequestIDs {
		requestIDs = requestIDs[:maxRequestIDs]
	}
	data, err := json.MarshalIndent(struct {
		RequestIDs []string `json:"request_ids"`
		Total      int      `json:"total"`
		Truncated  bool     `json:"truncated"`
	}{RequestIDs: requestIDs, Total: total, Truncated: total > maxRequestIDs}, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal request IDs: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

// handleGetRequestDetails handles the get_request_details tool call
func (h *HARServer) handleGetRequestDetails(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.harData == nil {
		return mcp.NewToolResultError("No HAR file loaded. Please load a HAR file first using load_har."), nil
	}

	var args struct {
		RequestID string `json:"request_id"`
	}
	if err := request.BindArguments(&args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	details, err := h.parser.GetRequestDetails(h.harData, args.RequestID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error getting request details: %v", err)), nil
	}

	data, err := json.MarshalIndent(details, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal request details: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

// handleGetResponseBody handles the get_response_body tool call
func (h *HARServer) handleGetResponseBody(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.harData == nil {
		return mcp.NewToolResultError("No HAR file loaded. Please load a HAR file first using load_har."), nil
	}

	var args struct {
		Hash   string `json:"hash"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := request.BindArguments(&args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	chunk, err := h.parser.GetResponseBody(args.Hash, args.Offset, args.Limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error getting response body: %v", err)), nil
	}

	data, err := json.MarshalIndent(chunk, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response body: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func main() {
	// Create the HAR server
	harServer := NewHARServer()

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"har-mcp",
		"1.0.0",
	)

	// Add tools
	mcpServer.AddTools(harServer.createTools()...)

	// Create and start stdio server
	stdioServer := server.NewStdioServer(mcpServer)

	log.Println("Starting HAR MCP server...")
	if err := stdioServer.Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatal("Server error:", err)
	}
}
