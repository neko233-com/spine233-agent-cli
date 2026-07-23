# spine233-agent-cli

Go 1.26 local CLI and stdio MCP for licensed Spine Pro workflows.

- Run `gofmt -w .`, `go vet ./...`, and `go test ./...` before publishing.
- Keep stdout machine-readable. MCP uses stdio JSON-RPC only.
- Do not add networking, telemetry, or unrequested file writes.
- JSON patch must remain preview-first and never overwrite its input.
- Semantic `.spine` conversion must use `spine233-file-parser`.
