// Package cli implements machine-readable command-line entrypoints.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/neko233-com/spine233-agent-cli/internal/app"
	"github.com/neko233-com/spine233-agent-cli/internal/mcp"
	"github.com/neko233-com/spine233-agent-cli/internal/spineops"
)

// Run executes one CLI command.
func Run(ctx context.Context, args []string, input io.Reader, output, errorOutput io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := io.WriteString(output, usage)
		return err
	}
	switch args[0] {
	case "serve", "mcp", "--stdio":
		return mcp.Serve(ctx, input, output)
	case "version", "--version":
		return printJSON(output, map[string]string{"name": "spine233-agent-cli", "version": app.Version, "go": "1.26"})
	case "detect":
		flags := newFlags("detect", errorOutput)
		path := flags.String("file", "", "local Spine file")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.Detect(*path)
		return printJSONMust(output, value, err)
	case "summarize":
		flags := newFlags("summarize", errorOutput)
		path := flags.String("file", "", "local Spine file")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.Summarize(*path)
		return printJSONMust(output, value, err)
	case "inspect":
		flags := newFlags("inspect", errorOutput)
		path := flags.String("file", "", "local .spine project")
		outputDirectory := flags.String("output-dir", "", "diagnostics directory")
		omitDecoded := flags.Bool("omit-decoded", false, "omit decoded private payload")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.Inspect(spineops.InspectOptions{
			Path: *path, OutputDirectory: *outputDirectory, OmitDecodedBinary: *omitDecoded,
		})
		return printJSONMust(output, value, err)
	case "export":
		flags := newFlags("export", errorOutput)
		path := flags.String("file", "", "local .spine project")
		outputDirectory := flags.String("output-dir", "", "export directory")
		executable := flags.String("spine", "", "licensed Spine executable")
		settings := flags.String("settings", "", "Spine export settings JSON")
		editorVersion := flags.String("editor-version", "", "Spine Editor version")
		timeout := flags.Duration("timeout", 2*time.Minute, "Spine CLI timeout")
		omitDecoded := flags.Bool("omit-decoded", false, "omit decoded private payload")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.Export(ctx, spineops.ExportOptions{
			Path: *path, OutputDirectory: *outputDirectory, Executable: *executable,
			ExportSettings: *settings, EditorVersion: *editorVersion, Timeout: *timeout,
			OmitDecodedBinary: *omitDecoded,
		})
		return printJSONMust(output, value, err)
	case "import":
		flags := newFlags("import", errorOutput)
		jsonPath := flags.String("json", "", "local Spine JSON input")
		projectPath := flags.String("output", "", "output .spine project")
		outputDirectory := flags.String("diagnostics-dir", "", "diagnostics directory")
		executable := flags.String("spine", "", "licensed Spine executable")
		editorVersion := flags.String("editor-version", "", "Spine Editor version")
		skeletonName := flags.String("skeleton-name", "", "imported skeleton name")
		timeout := flags.Duration("timeout", 2*time.Minute, "Spine CLI timeout")
		overwrite := flags.Bool("overwrite", false, "allow replacing existing output project")
		omitDecoded := flags.Bool("omit-decoded", false, "omit decoded private payload")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.Import(ctx, spineops.ImportOptions{
			JSONPath: *jsonPath, ProjectPath: *projectPath, OutputDirectory: *outputDirectory,
			Executable: *executable, EditorVersion: *editorVersion, SkeletonName: *skeletonName,
			Timeout: *timeout, Overwrite: *overwrite, OmitDecodedBinary: *omitDecoded, Indent: "  ",
		})
		return printJSONMust(output, value, err)
	case "query":
		flags := newFlags("query", errorOutput)
		path := flags.String("file", "", "local Spine JSON")
		pointer := flags.String("pointer", "", "RFC 6901 JSON Pointer")
		maxBytes := flags.Int("max-bytes", 1024*1024, "maximum encoded result bytes")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.QueryJSON(*path, *pointer, *maxBytes)
		return printJSONMust(output, value, err)
	case "patch":
		flags := newFlags("patch", errorOutput)
		inputPath := flags.String("file", "", "local Spine JSON input")
		outputPath := flags.String("output", "", "new Spine JSON output")
		patches := flags.String("operations", "", "JSON array of add/replace/remove operations")
		apply := flags.Bool("apply", false, "write output; otherwise preview")
		overwrite := flags.Bool("overwrite", false, "allow replacing existing output")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		var operations []spineops.PatchOperation
		if err := json.Unmarshal([]byte(*patches), &operations); err != nil {
			return fmt.Errorf("parse --operations: %w", err)
		}
		value, err := spineops.PatchJSON(spineops.PatchOptions{
			InputPath: *inputPath, OutputPath: *outputPath, Operations: operations,
			Apply: *apply, Overwrite: *overwrite, Indent: "  ",
		})
		return printJSONMust(output, value, err)
	default:
		return fmt.Errorf("unknown command %q; run spine233-agent-cli help", args[0])
	}
}

func newFlags(name string, errorOutput io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	return flags
}

func printJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printJSONMust[T any](output io.Writer, value T, err error) error {
	if err != nil {
		return err
	}
	return printJSON(output, value)
}

// IsUsageError identifies errors appropriate for exit code 2.
func IsUsageError(err error) bool {
	return errors.Is(err, flag.ErrHelp) || strings.Contains(err.Error(), "unknown command")
}

const usage = `spine233-agent-cli - local Spine Pro CLI and MCP

Usage:
  spine233-agent-cli serve
  spine233-agent-cli detect    --file character.spine
  spine233-agent-cli summarize --file character.json
  spine233-agent-cli inspect   --file character.spine [--output-dir DIR]
  spine233-agent-cli export    --file character.spine [--spine PATH] [--output-dir DIR]
  spine233-agent-cli import    --json character.json --output character.spine [--spine PATH]
  spine233-agent-cli query     --file character.json --pointer /animations/walk
  spine233-agent-cli patch     --file character.json --operations JSON [--output FILE --apply]
  spine233-agent-cli version

SPINE_EXECUTABLE may provide licensed Spine.com/Spine path.
All command output is JSON. serve/mcp/--stdio starts MCP over stdin/stdout.
`

// Main is the process adapter.
func Main() {
	if err := Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"error": err.Error()})
		if IsUsageError(err) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
