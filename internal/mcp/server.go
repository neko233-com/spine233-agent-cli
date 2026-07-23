// Package mcp implements line-delimited stdio MCP JSON-RPC transport.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/neko233-com/spine233-agent-cli/internal/app"
	"github.com/neko233-com/spine233-agent-cli/internal/spineops"
)

const protocolVersion = "2025-06-18"

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve reads newline-delimited MCP requests until EOF.
func Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			if err := encoder.Encode(response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}}); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue
		}
		if err := encoder.Encode(dispatch(ctx, req)); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read MCP stdin: %w", err)
	}
	return nil
}

func dispatch(ctx context.Context, req request) response {
	result := response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		result.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]string{"name": "spine233-agent-cli", "version": app.Version},
			"instructions":    "Local Spine Pro tools. Read-only by default. Export/import require a licensed Spine executable. JSON patches require apply=true and never overwrite input.",
		}
	case "ping":
		result.Result = map[string]any{}
	case "tools/list":
		result.Result = map[string]any{"tools": tools()}
	case "tools/call":
		value, err := callTool(ctx, req.Params)
		if err != nil {
			result.Result = toolError(err)
		} else {
			result.Result = toolResult(value)
		}
	default:
		result.Error = &rpcError{Code: -32601, Message: "method not found"}
	}
	return result
}

func tools() []map[string]any {
	path := map[string]any{"type": "string", "description": "Local .spine, .skel, or Spine JSON file path"}
	executable := map[string]any{"type": "string", "description": "Licensed Spine executable; defaults to SPINE_EXECUTABLE, Spine.com, or Spine"}
	timeout := map[string]any{"type": "integer", "minimum": 1, "maximum": 3600, "default": 120}
	return []map[string]any{
		{
			"name": "spine_detect", "description": "Detect local .spine, .skel, or Spine JSON format without launching Spine Editor.",
			"inputSchema": schema(map[string]any{"path": path}, "path"),
		},
		{
			"name": "spine_summarize", "description": "Summarize project metadata or JSON bones, slots, skins, events, and animations. Private .spine semantic data requires spine_export_project.",
			"inputSchema": schema(map[string]any{"path": path}, "path"),
		},
		{
			"name": "spine_inspect_project", "description": "Inspect private .spine metadata and write retained diagnostics without launching Spine Editor.",
			"inputSchema": schema(map[string]any{
				"path":              path,
				"outputDirectory":   map[string]any{"type": "string", "description": "Optional diagnostics directory; unique temp directory when omitted"},
				"omitDecodedBinary": map[string]any{"type": "boolean", "default": false},
			}, "path"),
		},
		{
			"name": "spine_export_project", "description": "Use installed licensed Spine Pro CLI to export semantic .spine data to JSON. Returns compact summaries and local output paths.",
			"inputSchema": schema(map[string]any{
				"path":              path,
				"outputDirectory":   map[string]any{"type": "string"},
				"executable":        executable,
				"exportSettings":    map[string]any{"type": "string", "description": "Spine export settings JSON path; defaults to built-in json"},
				"editorVersion":     map[string]any{"type": "string"},
				"timeoutSeconds":    timeout,
				"omitDecodedBinary": map[string]any{"type": "boolean", "default": false},
			}, "path"),
		},
		{
			"name": "spine_import_project", "description": "Use installed licensed Spine Pro CLI to import semantic Spine JSON as a .spine project. Refuses existing output unless overwrite=true.",
			"inputSchema": schema(map[string]any{
				"jsonPath":          map[string]any{"type": "string", "description": "Local Spine JSON input"},
				"projectPath":       map[string]any{"type": "string", "description": "Output .spine path"},
				"outputDirectory":   map[string]any{"type": "string", "description": "Optional diagnostics directory"},
				"executable":        executable,
				"editorVersion":     map[string]any{"type": "string"},
				"skeletonName":      map[string]any{"type": "string"},
				"timeoutSeconds":    timeout,
				"omitDecodedBinary": map[string]any{"type": "boolean", "default": false},
				"overwrite":         map[string]any{"type": "boolean", "default": false},
			}, "jsonPath", "projectPath"),
		},
		{
			"name": "spine_query_json", "description": "Read a bounded semantic subtree from Spine JSON using RFC 6901 JSON Pointer, for example /animations/walk.",
			"inputSchema": schema(map[string]any{
				"path":     map[string]any{"type": "string", "description": "Local Spine JSON path"},
				"pointer":  map[string]any{"type": "string", "description": "RFC 6901 pointer; empty selects root"},
				"maxBytes": map[string]any{"type": "integer", "minimum": 1, "maximum": 16777216, "default": 1048576, "description": "Reject larger results; use a narrower pointer"},
			}, "path", "pointer"),
		},
		{
			"name": "spine_patch_json", "description": "Preview or apply add/replace/remove operations to Spine JSON. apply=false previews. apply=true requires a different outputPath and never overwrites input.",
			"inputSchema": schema(map[string]any{
				"inputPath":  map[string]any{"type": "string"},
				"outputPath": map[string]any{"type": "string"},
				"operations": map[string]any{
					"type": "array", "minItems": 1,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"op":    map[string]any{"type": "string", "enum": []string{"add", "replace", "remove"}},
							"path":  map[string]any{"type": "string"},
							"value": map[string]any{},
						},
						"required": []string{"op", "path"},
					},
				},
				"apply":     map[string]any{"type": "boolean", "default": false},
				"overwrite": map[string]any{"type": "boolean", "default": false},
			}, "inputPath", "operations"),
		},
	}
}

func schema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

type toolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func callTool(ctx context.Context, raw json.RawMessage) (any, error) {
	var call toolCall
	if err := json.Unmarshal(raw, &call); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}
	switch call.Name {
	case "spine_detect":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.Detect(args.Path)
	case "spine_summarize":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.Summarize(args.Path)
	case "spine_inspect_project":
		var args spineops.InspectOptions
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.Inspect(args)
	case "spine_export_project":
		var args spineops.ExportOptions
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.Export(ctx, args)
	case "spine_import_project":
		var args spineops.ImportOptions
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.Import(ctx, args)
	case "spine_query_json":
		var args struct {
			Path     string `json:"path"`
			Pointer  string `json:"pointer"`
			MaxBytes int    `json:"maxBytes"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		if args.MaxBytes == 0 {
			return spineops.QueryJSON(args.Path, args.Pointer)
		}
		return spineops.QueryJSON(args.Path, args.Pointer, args.MaxBytes)
	case "spine_patch_json":
		var args spineops.PatchOptions
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.PatchJSON(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", call.Name)
	}
}

func toolResult(value any) map[string]any {
	encoded, encodeErr := json.MarshalIndent(value, "", "  ")
	if encodeErr != nil {
		return toolError(encodeErr)
	}
	return map[string]any{
		"content":           []map[string]string{{"type": "text", "text": string(encoded)}},
		"structuredContent": value,
	}
}

func toolError(err error) map[string]any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": err.Error()}},
		"isError": true,
	}
}
