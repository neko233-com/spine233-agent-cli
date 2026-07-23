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
			"instructions":    "Local Spine Pro tools. Direct .spine and JSON edits are preview-first, require apply=true to write, and never overwrite input. No Spine executable is invoked. Agent workflow: list animations and transform timelines, build a filtered recipe, edit keys, preview rewrite, apply to a new *-agent.spine path, then compare human and agent animations.",
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
	return []map[string]any{
		{
			"name": "spine_detect", "description": "Detect local .spine, .skel, or Spine JSON format without launching Spine Editor.",
			"inputSchema": schema(map[string]any{"path": path}, "path"),
		},
		{
			"name": "spine_summarize", "description": "Summarize project metadata or JSON bones, slots, skins, events, and animations without invoking Spine Editor.",
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
			"name": "spine_list_project_animations", "description": "Directly decode .spine top-level animation names, count, offsets, and record boundaries without Spine Editor.",
			"inputSchema": schema(map[string]any{"path": path}, "path"),
		},
		{
			"name": "spine_list_project_bones", "description": "Directly decode .spine bone names, serialization offsets, and raw Kryo parent tokens without Spine Editor.",
			"inputSchema": schema(map[string]any{"path": path}, "path"),
		},
		{
			"name": "spine_list_project_rotate_timelines", "description": "Decode one .spine animation's rotate timelines, bone references, frame numbers, values, curves, and exact offsets without Spine Editor.",
			"inputSchema": schema(map[string]any{
				"path":      path,
				"animation": map[string]any{"type": "string", "description": "Unique top-level animation record name"},
			}, "path", "animation"),
		},
		{
			"name": "spine_list_project_transform_timelines", "description": "Decode one .spine animation's rotate, translate, scale, and shear timelines with bone references, frames, channel values, curves, and offsets.",
			"inputSchema": schema(map[string]any{
				"path":      path,
				"animation": map[string]any{"type": "string", "description": "Unique top-level animation record name"},
			}, "path", "animation"),
		},
		{
			"name": "spine_compare_project_transform_animation", "description": "Read-only semantic comparison of human and agent .spine animations. Verifies {animation}-agent naming, fixed topology, and changed frames, values, or curves.",
			"inputSchema": schema(map[string]any{
				"sourcePath":      path,
				"sourceAnimation": map[string]any{"type": "string"},
				"targetPath":      path,
				"targetAnimation": map[string]any{"type": "string"},
				"maxChanges": map[string]any{
					"type":    "integer",
					"minimum": 1,
					"maximum": 100000,
					"default": 1000,
				},
			}, "sourcePath", "sourceAnimation", "targetPath", "targetAnimation"),
		},
		{
			"name": "spine_build_project_transform_recipe", "description": "Build a full or filtered editable rewrite recipe from an existing .spine animation. Defaults target to {animation}-agent and output to sibling *-agent.spine. Does not write.",
			"inputSchema": schema(map[string]any{
				"path":            path,
				"animation":       map[string]any{"type": "string"},
				"targetAnimation": map[string]any{"type": "string"},
				"outputPath":      map[string]any{"type": "string"},
				"includeCurves":   map[string]any{"type": "boolean", "default": false},
				"boneReferences": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "integer", "minimum": 0},
					"description": "Optional Kryo bone references; empty selects all",
				},
				"timelineTypes": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
						"enum": []string{"rotate", "translate", "scale", "shear"},
					},
					"description": "Optional transform types; empty selects all",
				},
			}, "path", "animation"),
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
		{
			"name": "spine_analyze_json", "description": "Deep inventory of bones, slots, skins, animations, events, all constraint families, attachment types, and timeline types.",
			"inputSchema": schema(map[string]any{"path": map[string]any{"type": "string", "description": "Local Spine JSON path"}}, "path"),
		},
		{
			"name": "spine_validate_json", "description": "Validate stable cross-version bone, slot, constraint, and animation references without writing.",
			"inputSchema": schema(map[string]any{"path": map[string]any{"type": "string", "description": "Local Spine JSON path"}}, "path"),
		},
		{
			"name": "spine_clone_animation", "description": "Preview or generate an animation by cloning and retiming a source, replacing selected bone rotate/translate/scale/shear timelines, and adding an Agent marker event.",
			"inputSchema": schema(map[string]any{
				"inputPath":       map[string]any{"type": "string"},
				"outputPath":      map[string]any{"type": "string"},
				"source":          map[string]any{"type": "string"},
				"target":          map[string]any{"type": "string"},
				"timeScale":       map[string]any{"type": "number", "exclusiveMinimum": 0, "default": 1},
				"boneMotions":     map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"markerEvent":     map[string]any{"type": "string", "default": "agent-generated"},
				"replaceExisting": map[string]any{"type": "boolean", "default": false},
				"apply":           map[string]any{"type": "boolean", "default": false},
				"overwrite":       map[string]any{"type": "boolean", "default": false},
			}, "inputPath", "source", "target"),
		},
		{
			"name": "spine_patch_project_animation", "description": "Preview or apply exact float32 keyframe edits and optional animation rename directly inside one .spine record. Never invokes Spine Editor or overwrites input.",
			"inputSchema": schema(map[string]any{
				"inputPath":       map[string]any{"type": "string"},
				"outputPath":      map[string]any{"type": "string"},
				"animation":       map[string]any{"type": "string"},
				"targetAnimation": map[string]any{"type": "string", "description": "Optional renamed animation, convention: {animation}-agent"},
				"endBefore":       map[string]any{"type": "string", "description": "Next animation record name; omit for final record"},
				"edits": map[string]any{
					"type": "array", "minItems": 1,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"from":            map[string]any{"type": "number"},
							"to":              map[string]any{"type": "number"},
							"expectedMatches": map[string]any{"type": "integer", "minimum": 1},
						},
						"required": []string{"from", "to", "expectedMatches"},
					},
				},
				"apply":     map[string]any{"type": "boolean", "default": false},
				"overwrite": map[string]any{"type": "boolean", "default": false},
			}, "inputPath", "animation", "edits"),
		},
		{
			"name": "spine_patch_project_rotate", "description": "Preview or apply semantic rotate-key edits selected by bone reference, key index, and expected old value. Can rename to {animation}-agent; never invokes Spine Editor or overwrites input.",
			"inputSchema": schema(map[string]any{
				"inputPath":       map[string]any{"type": "string"},
				"outputPath":      map[string]any{"type": "string"},
				"animation":       map[string]any{"type": "string"},
				"targetAnimation": map[string]any{"type": "string", "description": "Optional renamed animation, convention: {animation}-agent"},
				"edits": map[string]any{
					"type": "array", "minItems": 1,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"boneReference": map[string]any{"type": "integer", "minimum": 1},
							"keyIndex":      map[string]any{"type": "integer", "minimum": 0},
							"from":          map[string]any{"type": "number"},
							"to":            map[string]any{"type": "number"},
						},
						"required": []string{"boneReference", "keyIndex", "from", "to"},
					},
				},
				"apply":     map[string]any{"type": "boolean", "default": false},
				"overwrite": map[string]any{"type": "boolean", "default": false},
			}, "inputPath", "animation", "edits"),
		},
		{
			"name": "spine_patch_project_transform", "description": "Preview or apply semantic rotate/translate/scale/shear channel edits selected by bone reference, timeline, key index, channel, and old value. Never invokes Spine Editor or overwrites input.",
			"inputSchema": schema(map[string]any{
				"inputPath":       map[string]any{"type": "string"},
				"outputPath":      map[string]any{"type": "string"},
				"animation":       map[string]any{"type": "string"},
				"targetAnimation": map[string]any{"type": "string", "description": "Optional renamed animation, convention: {animation}-agent"},
				"edits": map[string]any{
					"type": "array", "minItems": 1,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"boneReference": map[string]any{"type": "integer", "minimum": 1},
							"timeline":      map[string]any{"type": "string", "enum": []string{"rotate", "translate", "scale", "shear"}},
							"keyIndex":      map[string]any{"type": "integer", "minimum": 0},
							"channel": map[string]any{
								"type":    "string",
								"pattern": `^(frame|value|x|y|curve\.(value|x|y)\.[0-3])$`,
							},
							"from": map[string]any{"type": "number"},
							"to":   map[string]any{"type": "number"},
						},
						"required": []string{"boneReference", "timeline", "keyIndex", "channel", "from", "to"},
					},
				},
				"apply":     map[string]any{"type": "boolean", "default": false},
				"overwrite": map[string]any{"type": "boolean", "default": false},
			}, "inputPath", "animation", "edits"),
		},
		{
			"name": "spine_rewrite_project_transform_animation", "description": "Preview or apply complete fixed-topology transform timeline declarations. Rewrites frame/value/curve data and can rename to {animation}-agent without changing Kryo object counts.",
			"inputSchema": schema(map[string]any{
				"inputPath":       map[string]any{"type": "string"},
				"outputPath":      map[string]any{"type": "string"},
				"animation":       map[string]any{"type": "string"},
				"targetAnimation": map[string]any{"type": "string", "description": "Optional renamed animation, convention: {animation}-agent"},
				"timelines": map[string]any{
					"type": "array", "minItems": 1,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"boneReference": map[string]any{"type": "integer", "minimum": 1},
							"timeline":      map[string]any{"type": "string", "enum": []string{"rotate", "translate", "scale", "shear"}},
							"keys": map[string]any{
								"type": "array", "minItems": 1,
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"frame":  map[string]any{"type": "number", "minimum": 0},
										"values": map[string]any{"type": "array", "minItems": 1, "maxItems": 2, "items": map[string]any{"type": "number"}},
										"curves": map[string]any{"type": "array", "items": map[string]any{
											"type": "array", "minItems": 4, "maxItems": 4, "items": map[string]any{"type": "number"},
										}},
									},
									"required": []string{"frame", "values"},
								},
							},
						},
						"required": []string{"boneReference", "timeline", "keys"},
					},
				},
				"apply":     map[string]any{"type": "boolean", "default": false},
				"overwrite": map[string]any{"type": "boolean", "default": false},
			}, "inputPath", "animation", "timelines"),
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
	case "spine_list_project_animations":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.ListProjectAnimations(args.Path)
	case "spine_list_project_bones":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.ListProjectBones(args.Path)
	case "spine_list_project_rotate_timelines":
		var args struct {
			Path      string `json:"path"`
			Animation string `json:"animation"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.ListProjectRotateTimelines(args.Path, args.Animation)
	case "spine_list_project_transform_timelines":
		var args struct {
			Path      string `json:"path"`
			Animation string `json:"animation"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.ListProjectTransformTimelines(args.Path, args.Animation)
	case "spine_compare_project_transform_animation":
		var args spineops.ProjectTransformComparisonOptions
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.CompareProjectTransformAnimations(args)
	case "spine_build_project_transform_recipe":
		var args struct {
			Path            string   `json:"path"`
			Animation       string   `json:"animation"`
			TargetAnimation string   `json:"targetAnimation"`
			OutputPath      string   `json:"outputPath"`
			IncludeCurves   bool     `json:"includeCurves"`
			BoneReferences  []int    `json:"boneReferences"`
			TimelineTypes   []string `json:"timelineTypes"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.BuildProjectTransformRecipeWithOptions(
			spineops.ProjectTransformRecipeBuildOptions{
				Path:            args.Path,
				Animation:       args.Animation,
				TargetAnimation: args.TargetAnimation,
				OutputPath:      args.OutputPath,
				IncludeCurves:   args.IncludeCurves,
				BoneReferences:  args.BoneReferences,
				TimelineTypes:   args.TimelineTypes,
			},
		)
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
	case "spine_analyze_json":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.Analyze(args.Path)
	case "spine_validate_json":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.Validate(args.Path)
	case "spine_clone_animation":
		var args spineops.AnimationOptions
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.GenerateAnimation(args)
	case "spine_patch_project_animation":
		var args spineops.ProjectAnimationOptions
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.PatchProjectAnimation(args)
	case "spine_patch_project_rotate":
		var args spineops.ProjectRotateOptions
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.PatchProjectRotate(args)
	case "spine_patch_project_transform":
		var args spineops.ProjectTransformOptions
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.PatchProjectTransform(args)
	case "spine_rewrite_project_transform_animation":
		var args spineops.ProjectTransformRewriteOptions
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, err
		}
		return spineops.RewriteProjectTransform(args)
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
