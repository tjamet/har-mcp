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

// NewHARServer creates a new HAR MCP server
func NewHARServer() *HARServer {
	return &HARServer{
		parser: harParser.NewParser(),
	}
}

// loadHAR loads a HAR file from the given source
func (h *HARServer) loadHAR(source string) error {
	harData, err := h.parser.ParseSource(source)
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
				Description: "Load a HAR file from a file path or HTTP URL",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"source": map[string]interface{}{
							"type":        "string",
							"description": "File path or HTTP URL to the HAR file",
						},
					},
					Required: []string{"source"},
				},
			},
			Handler: h.handleLoadHAR,
		},
		{
			Tool: mcp.Tool{
				Name:        "list_urls_methods",
				Description: "List all accessed URLs and their HTTP methods from the loaded HAR file",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}{},
				},
			},
			Handler: h.handleListURLsMethods,
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
							"description": "The URL to filter by",
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
				Description: "Get full request details by request ID (authentication headers will be redacted)",
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
	}
}

// handleLoadHAR handles the load_har tool call
func (h *HARServer) handleLoadHAR(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Source string `json:"source"`
	}
	if err := request.BindArguments(&args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	if err := h.loadHAR(args.Source); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error loading HAR file: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully loaded HAR file with %d entries", len(h.harData.Log.Entries))), nil
}

// handleListURLsMethods handles the list_urls_methods tool call
func (h *HARServer) handleListURLsMethods(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.harData == nil {
		return mcp.NewToolResultError("No HAR file loaded. Please load a HAR file first using load_har."), nil
	}

	entries := h.parser.GetURLsAndMethods(h.harData)
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal URLs and methods: %v", err)), nil
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
	data, err := json.MarshalIndent(requestIDs, "", "  ")
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
