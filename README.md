# HAR MCP Server

A Model Context Protocol (MCP) server for parsing and analyzing HAR (HTTP Archive) files. This server allows AI assistants to inspect network traffic captured in HAR format, with built-in support for redacting sensitive authentication headers.

## Features

- **Load HAR files** from local filesystem or HTTP URLs
- **List all URLs and HTTP methods** accessed in the HAR file
- **Query request IDs** for specific URL and method combinations
- **Retrieve full request details** with automatic redaction of authentication headers
- Support for standard HAR format as produced by browser developer tools

## Installation

```bash
go get github.com/tjamet/har-mcp
```

## Building

```bash
go build ./cmd/har-mcp
```

## Usage

The HAR MCP server runs as a stdio-based MCP server, communicating via JSON-RPC over standard input/output.

### Running the Server

```bash
./har-mcp
```

### Available Tools

#### 1. `load_har`
Load a HAR file from a file path or HTTP URL.

**Parameters:**
- `source` (string, required): File path or HTTP URL to the HAR file

**Example:**
```json
{
  "source": "/path/to/capture.har"
}
```

#### 2. `list_urls_methods`
List all accessed URLs and their HTTP methods from the loaded HAR file.

**Parameters:** None

**Returns:** Array of URL/method combinations with their associated request IDs.

#### 3. `get_request_ids`
Get all request IDs for a specific URL and HTTP method.

**Parameters:**
- `url` (string, required): The URL to filter by
- `method` (string, required): The HTTP method to filter by (GET, POST, etc.)

**Example:**
```json
{
  "url": "https://api.example.com/users",
  "method": "GET"
}
```

#### 4. `get_request_details`
Get full request details by request ID. Authentication headers will be automatically redacted.

**Parameters:**
- `request_id` (string, required): The request ID to retrieve details for

**Example:**
```json
{
  "request_id": "request_0"
}
```

**Redacted Headers:**
- Authorization
- X-API-Key
- X-Auth-Token
- Cookie
- Set-Cookie
- Proxy-Authorization

## Integration with Claude Desktop

Add the following to your Claude Desktop configuration:

```json
{
  "mcpServers": {
    "har-mcp": {
      "command": "/path/to/har-mcp"
    }
  }
}
```

## Development

### Running Tests

```bash
go test ./...
```

### Project Structure

```
.
├── cmd/
│   └── har-mcp/       # Main application
│       └── main.go
├── pkg/
│   └── har/           # HAR parsing library
│       ├── parser.go
│       └── parser_test.go
├── go.mod
├── go.sum
└── README.md
```

## Dependencies

- [github.com/google/martian/har](https://github.com/google/martian) - HAR file parsing
- [github.com/mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) - MCP server implementation
- [github.com/stretchr/testify](https://github.com/stretchr/testify) - Testing assertions

## License

[Add your license here] 