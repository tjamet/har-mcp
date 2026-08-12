# HAR MCP Server

A Model Context Protocol (MCP) stdio server for analyzing HAR (HTTP Archive) files.
Exposes 5 tools — `load_har`, `list_entries`, `get_request_ids`, `get_request_details`,
`get_response_body` — with auth-header redaction and context-bounded bodies. Parsing
uses `github.com/google/martian/har`; the MCP layer uses `github.com/mark3labs/mcp-go`;
tests use `github.com/stretchr/testify`.

## Layout

- `cmd/har-mcp/main.go` — MCP server entrypoint, tool definitions, tool handlers
- `pkg/har/parser.go` — Parser: source detection, parse fallback chain, queries, redaction
- `pkg/har/body_store.go` — BodyStore: SHA-256 content-addressed storage of response/request bodies
- `pkg/har/custom_types.go` — FlexibleHAR types: float time fields, base64/plain content
- `pkg/har/parser_test.go` — tests (testify, helpers, no table-driven tests)
- `example.har` — sample capture for manual testing

## Dev environment

Go 1.24.3 (go.mod, CI, and Dockerfile all pin it). No Makefile, no vendoring.

## Build & test

```sh
go build ./cmd/har-mcp      # builds ./har-mcp (gitignored — never commit it)
go run ./cmd/har-mcp        # runs; blocks on stdin, see Pitfalls
go test ./...               # unit tests (README + local)
go test -v ./...            # CI form (pr.yaml)
go vet ./...                # static checks
go fmt ./...                # formatting
go mod tidy                 # deps; keep go.mod/go.sum minimal and pinned
```

CI (`.github/workflows/pr.yaml`) additionally runs `golangci-lint` (latest) and a
multi-arch docker build. `build.yaml` builds/pushes `ghcr.io/tjamet/har-mcp` on
push to `main` or `v*` tags. Dockerfile is multi-stage, `CGO_ENABLED=0`, alpine runtime.

## Conventions

- Follow `.cursor/rules/go.mdc` (repo rule file): `cmd/<prog>/main.go` for binaries,
  `pkg/**` for libraries; `fmt.Errorf("context: %w", err)` everywhere; godoc comments on
  all exported symbols; comments explain WHY.
- Tests: `require` for setup that must succeed, `assert` for assertions, helper funcs
  to build fixtures (`createTestHAR`, `parseTestHAR`, `createResponseHAR`,
  `createResponseHARWithParser`, `createPostDataHAR`), one behavior per test, NO
  table-driven tests.
- Handlers never return errors to the client: bind/parse failures are returned as
  `mcp.NewToolResultError(...)` with a `nil` error.
- Tool outputs are CONTEXT-BOUNDED — this is load-bearing, do not bypass it: response
  bodies and request postData are stored in the `BodyStore` at parse time and never
  serialized raw. Details calls return at most a 4KB `textPreview` (bodies ≤4KB are
  fully inlined; larger ones are cut at a rune boundary, never mid-rune, and end with
  a `[TRUNCATED]` marker so the cut reads as intentional rather than broken JSON) plus
  a `body:<64 hex sha256>` content hash; `get_response_body` fetches the full body in
  chunks. `list_entries` URLs keep query params — they discriminate requests — capped
  at 100 chars; sensitive query values are redacted.
- Preview textability falls back to a UTF-8 content sniff (`looksLikeText`) when the
  declared mime type is missing or wrong (e.g. JSON served as
  `application/octet-stream`); bodies that fail both the mime allowlist and the sniff
  are metadata-only. The sniff is a heuristic, not a verdict.
- `Parser` holds the `BodyStore` and the `LoadPolicy` of the most recent parse (it is
  NOT stateless); query methods take `*har.HAR` as an argument. `HARServer` in main.go
  holds the loaded `*har.HAR` state.
- Parsing is two-pass: standard `har.HAR` decode first, fall back to `FlexibleHAR`
  (float time fields truncated to `int64`, content.text as string or base64). The body
  store and policy reset on every successful parse, so a new load replaces the
  previous one (old hashes are no longer fetchable).

## Pitfalls

- Request IDs are POSITIONAL indices, not stable identifiers: `request_%d` is the index
  into `harData.Log.Entries`. `GetRequestDetails` parses the suffix and bounds-checks it
  (`fmt.Sscanf(requestID, "request_%d", &index)`). Reordering/inserting entries renumbers
  every ID. `GetEntries`/`GetURLsAndMethods` skip entries with nil `Request` but still
  count their index, so IDs can skip.
- Redaction covers BOTH request and response headers in `get_request_details`
  (`redactAuthHeaders`). `Set-Cookie` on responses is redacted. Sensitive QUERY
  values (token, api_key, session, ...) are redacted too — in index URLs, details
  URLs, and `queryString` — and URL matching (`list_entries` filter,
  `get_request_ids`) runs on the normalized, redacted form so displayed URLs
  round-trip. Redaction is name-list based; body VALUES are not redacted (response
  bodies are hash-gated behind `get_response_body`).
- The server is stdio-only JSON-RPC: running `./har-mcp` bare just blocks on stdin
  (normal MCP behavior). There are no flags, no HTTP mode. Manual testing requires an MCP
  client, or exercise the parser via `go test ./...` / a tiny Go program.
- `load_har` accepts a path or an http(s) URL (`ParseSource` sniffs the scheme); anything
  else is treated as a file path. URL fetches use plain `http.Get` (no timeouts, no TLS
  pinning). An optional `policy` (`excludeMimeTypes`, `maxKeepBytes`) keeps matching
  bodies out of the body store — excluded bodies still appear in details (previews
  work from the in-memory HAR) but get no hash and cannot be fetched.
- The built binary `/har-mcp` is gitignored; `go build ./cmd/har-mcp` from the repo root
  overwrites the stale local copy.
