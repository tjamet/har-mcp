# HAR MCP Server

A [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction) server for parsing and analyzing HAR (HTTP Archive) files. This server allows AI assistants to inspect network traffic captured in HAR format, with built-in support for redacting sensitive authentication headers.

## Features

- **Load HAR files** from local filesystem or HTTP URLs
- **List all entries** as one compact row each (method, status, mime type, size, timing, body hash), with substring filtering and pagination
- **Query request IDs** for specific URL and method combinations
- **Retrieve full request details** with automatic redaction of authentication headers
- **Flexible HAR parsing** that handles real-world HAR files with:
  - Float/decimal values for time fields (automatically rounded to integers)
  - Plain text or base64-encoded response content
  - Additional fields not present in the basic HAR spec
- Support for standard HAR format as produced by browser developer tools

## Installation

You can install this MCP server using your standard [MCP](https://modelcontextprotocol.io/introduction) configuration.

Add the following JSON block to your mcp configuration.

## Using docker

```json
{
  "mcp": {
    "servers": {
      "har": {
        "command": "docker",
        "args": [
          "run",
          "-i",
          "--rm",
          "ghcr.io/tjamet/har-mcp"
        ]
      }
    }
  }
}
```

## Using go run

Alternatively you can run thr server directly with `go run`.

```json
{
  "mcpServers": {
    "github": {
      "command": "go",
      "args": [
        "run",
        "github.com/tjamet/har-mcp/cmd/har-mcp@main"
      ]
    }
  }
}
```

### Build from source

If you don't have Docker, you can use `go build` to build the binary in the
`cmd/har-mcp` directory, and use the `github-mcp-server` command.
To specify the output location of the build, use the `-o` flag. You should configure your server to use the built executable as its `command`. For example:

```JSON
{
  "mcp": {
    "servers": {
      "github": {
        "command": "/path/to/har-mcp-server"
      }
    }
  }
}
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
- `policy` (object, optional): keeps matching response bodies out of the
  body store at load time. Bodies affected by the policy still appear in
  `get_request_details` (with previews) but get no `hash` and cannot be
  fetched via `get_response_body`:
  - `excludeMimeTypes` (string[], optional): mime type prefixes to exclude,
    case-insensitive (e.g. `"video/"`, `"image/*"`)
  - `maxKeepBytes` (number, optional): bodies larger than this many bytes
    are not stored (absent or `<= 0`: no limit)

**Example:**
```json
{
  "source": "/path/to/capture.har",
  "policy": {
    "excludeMimeTypes": ["video/", "image/*"],
    "maxKeepBytes": 1048576
  }
}
```

#### 2. `list_entries`
List all HAR entries as one compact row each — method, status, mime type, size,
timing, and body hash. Query params are kept — they discriminate requests —
but values for sensitive keys are redacted, and URLs are capped at 100 chars;
use `get_request_details` for the full URL. This is the primary index: call
this first, then `get_request_details`, then `get_response_body`.

**Parameters:**
- `filter` (string, optional): Case-insensitive substring match on the
  normalized request URL (query params included, sensitive values redacted)
- `method` (string, optional): The HTTP method to filter by (GET, POST, etc.)
- `offset` (number, optional, default 0): Row offset into the matching entries
- `limit` (number, optional, default 200, max 1000): Maximum number of rows to return

**Returns:** `{entries, total, offset, limit, truncated}` — one flat row per entry.

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

#### 5. `get_response_body`
Fetch a chunk of a stored response body by content hash. The hash is returned as
`response.content.hash` by `get_request_details`. Text bodies return the decoded bytes
between `offset` and `offset + limit`; binary bodies return metadata only.

**Parameters:**
- `hash` (string, required): Content hash reference of the body to fetch
- `offset` (number, optional, default 0): Byte offset into the decoded body
- `limit` (number, optional, default 4096): Maximum number of bytes to return (max 65536)

**Example:**

```json
{
  "hash": "body:3f2a1c9d8e7b6a5f4c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c",
  "offset": 0,
  "limit": 4096
}
```

## Client-side usage

- **Never attach a .har file raw to an agent context** (e.g. `@ capture.har`):
  a 5.7MB capture is roughly 1.4M tokens. The MCP tools are the only
  supported ingestion path: `load_har` -> `list_entries` ->
  `get_request_details` -> `get_response_body`.
- No MCP server available? Pre-trim the file with jq — strip all response
  bodies and postData text while keeping headers/status:
  `jq 'del(.log.entries[].response.content.text, .log.entries[].request.postData.text)' capture.har > trimmed.har`
- Or keep small bodies and only drop responses over 16KB:
  `jq '(.log.entries[] | select(.response.content.size > 16384) | .response.content.text) = null' capture.har > trimmed.har`

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
│       ├── body_store.go
│       ├── parser.go
│       ├── custom_types.go
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