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
	"path/filepath"
	"strings"

	"github.com/neko233-com/spine233-agent-cli/internal/app"
	"github.com/neko233-com/spine233-agent-cli/internal/mcp"
	"github.com/neko233-com/spine233-agent-cli/internal/spineops"
	spineparser "github.com/neko233-com/spine233-file-parser"
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
	case "animations":
		flags := newFlags("animations", errorOutput)
		path := flags.String("file", "", "local .spine project")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.ListProjectAnimations(*path)
		return printJSONMust(output, value, err)
	case "rotate-timelines":
		flags := newFlags("rotate-timelines", errorOutput)
		path := flags.String("file", "", "local .spine project")
		animation := flags.String("animation", "", "animation record name")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.ListProjectRotateTimelines(*path, *animation)
		return printJSONMust(output, value, err)
	case "transform-timelines":
		flags := newFlags("transform-timelines", errorOutput)
		path := flags.String("file", "", "local .spine project")
		animation := flags.String("animation", "", "animation record name")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.ListProjectTransformTimelines(*path, *animation)
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
	case "analyze":
		flags := newFlags("analyze", errorOutput)
		path := flags.String("file", "", "local Spine JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.Analyze(*path)
		return printJSONMust(output, value, err)
	case "validate":
		flags := newFlags("validate", errorOutput)
		path := flags.String("file", "", "local Spine JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := spineops.Validate(*path)
		return printJSONMust(output, value, err)
	case "animate":
		flags := newFlags("animate", errorOutput)
		inputPath := flags.String("json", "", "local Spine JSON input")
		outputPath := flags.String("output", "", "new Spine JSON output")
		source := flags.String("source", "", "source animation")
		target := flags.String("target", "", "target animation")
		timeScale := flags.Float64("time-scale", 1, "animation time multiplier")
		motions := flags.String("bone-motions", "[]", "JSON array of bone motions")
		marker := flags.String("marker-event", "agent-generated", "event added at time zero")
		apply := flags.Bool("apply", false, "write output; otherwise preview")
		overwrite := flags.Bool("overwrite", false, "allow replacing existing output")
		replaceExisting := flags.Bool("replace-existing", false, "allow replacing target animation")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		var boneMotions []spineparser.BoneMotion
		if err := json.Unmarshal([]byte(*motions), &boneMotions); err != nil {
			return fmt.Errorf("parse --bone-motions: %w", err)
		}
		value, err := spineops.GenerateAnimation(spineops.AnimationOptions{
			InputPath: *inputPath, OutputPath: *outputPath, Source: *source, Target: *target,
			TimeScale: *timeScale, BoneMotions: boneMotions, MarkerEvent: *marker,
			Apply: *apply, Overwrite: *overwrite, ReplaceExisting: *replaceExisting,
		})
		return printJSONMust(output, value, err)
	case "animate-project":
		flags := newFlags("animate-project", errorOutput)
		recipePath := flags.String("recipe", "", "project animation recipe JSON")
		inputPath := flags.String("file", "", "local .spine input")
		outputPath := flags.String("output", "", "new .spine output")
		animation := flags.String("animation", "", "animation record name")
		targetAnimation := flags.String("target-animation", "", "renamed output animation")
		endBefore := flags.String("end-before", "", "next animation record name")
		edits := flags.String("edits", "", "JSON array of exact float32 edits")
		apply := flags.Bool("apply", false, "write output; otherwise preview")
		overwrite := flags.Bool("overwrite", false, "allow replacing existing output")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		options := spineops.ProjectAnimationOptions{}
		if strings.TrimSpace(*recipePath) != "" {
			absoluteRecipe, err := filepath.Abs(*recipePath)
			if err != nil {
				return err
			}
			source, err := os.ReadFile(absoluteRecipe)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(source, &options); err != nil {
				return fmt.Errorf("parse --recipe: %w", err)
			}
			directory := filepath.Dir(absoluteRecipe)
			if options.InputPath != "" && !filepath.IsAbs(options.InputPath) {
				options.InputPath = filepath.Join(directory, options.InputPath)
			}
			if options.OutputPath != "" && !filepath.IsAbs(options.OutputPath) {
				options.OutputPath = filepath.Join(directory, options.OutputPath)
			}
		}
		if *inputPath != "" {
			options.InputPath = *inputPath
		}
		if *outputPath != "" {
			options.OutputPath = *outputPath
		}
		if *animation != "" {
			options.Animation = *animation
		}
		if *targetAnimation != "" {
			options.TargetAnimation = *targetAnimation
		}
		if *endBefore != "" {
			options.EndBefore = *endBefore
		}
		if strings.TrimSpace(*edits) != "" {
			if err := json.Unmarshal([]byte(*edits), &options.Edits); err != nil {
				return fmt.Errorf("parse --edits: %w", err)
			}
		}
		options.Apply = *apply
		options.Overwrite = *overwrite
		value, err := spineops.PatchProjectAnimation(options)
		return printJSONMust(output, value, err)
	case "animate-project-rotate":
		flags := newFlags("animate-project-rotate", errorOutput)
		recipePath := flags.String("recipe", "", "semantic rotate recipe JSON")
		inputPath := flags.String("file", "", "local .spine input")
		outputPath := flags.String("output", "", "new .spine output")
		animation := flags.String("animation", "", "animation record name")
		targetAnimation := flags.String("target-animation", "", "renamed output animation")
		edits := flags.String(
			"edits",
			"",
			"JSON array of boneReference/keyIndex/from/to edits",
		)
		apply := flags.Bool("apply", false, "write output; otherwise preview")
		overwrite := flags.Bool("overwrite", false, "allow replacing existing output")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		options := spineops.ProjectRotateOptions{}
		if strings.TrimSpace(*recipePath) != "" {
			absoluteRecipe, err := filepath.Abs(*recipePath)
			if err != nil {
				return err
			}
			source, err := os.ReadFile(absoluteRecipe)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(source, &options); err != nil {
				return fmt.Errorf("parse --recipe: %w", err)
			}
			directory := filepath.Dir(absoluteRecipe)
			if options.InputPath != "" && !filepath.IsAbs(options.InputPath) {
				options.InputPath = filepath.Join(directory, options.InputPath)
			}
			if options.OutputPath != "" && !filepath.IsAbs(options.OutputPath) {
				options.OutputPath = filepath.Join(directory, options.OutputPath)
			}
		}
		if *inputPath != "" {
			options.InputPath = *inputPath
		}
		if *outputPath != "" {
			options.OutputPath = *outputPath
		}
		if *animation != "" {
			options.Animation = *animation
		}
		if *targetAnimation != "" {
			options.TargetAnimation = *targetAnimation
		}
		if strings.TrimSpace(*edits) != "" {
			if err := json.Unmarshal([]byte(*edits), &options.Edits); err != nil {
				return fmt.Errorf("parse --edits: %w", err)
			}
		}
		options.Apply = *apply
		options.Overwrite = *overwrite
		value, err := spineops.PatchProjectRotate(options)
		return printJSONMust(output, value, err)
	case "animate-project-transform":
		flags := newFlags("animate-project-transform", errorOutput)
		recipePath := flags.String("recipe", "", "semantic transform recipe JSON")
		inputPath := flags.String("file", "", "local .spine input")
		outputPath := flags.String("output", "", "new .spine output")
		animation := flags.String("animation", "", "animation record name")
		targetAnimation := flags.String("target-animation", "", "renamed output animation")
		edits := flags.String(
			"edits",
			"",
			"JSON array of bone/timeline/key/channel edits",
		)
		apply := flags.Bool("apply", false, "write output; otherwise preview")
		overwrite := flags.Bool("overwrite", false, "allow replacing existing output")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		options := spineops.ProjectTransformOptions{}
		if strings.TrimSpace(*recipePath) != "" {
			absoluteRecipe, err := filepath.Abs(*recipePath)
			if err != nil {
				return err
			}
			source, err := os.ReadFile(absoluteRecipe)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(source, &options); err != nil {
				return fmt.Errorf("parse --recipe: %w", err)
			}
			directory := filepath.Dir(absoluteRecipe)
			if options.InputPath != "" && !filepath.IsAbs(options.InputPath) {
				options.InputPath = filepath.Join(directory, options.InputPath)
			}
			if options.OutputPath != "" && !filepath.IsAbs(options.OutputPath) {
				options.OutputPath = filepath.Join(directory, options.OutputPath)
			}
		}
		if *inputPath != "" {
			options.InputPath = *inputPath
		}
		if *outputPath != "" {
			options.OutputPath = *outputPath
		}
		if *animation != "" {
			options.Animation = *animation
		}
		if *targetAnimation != "" {
			options.TargetAnimation = *targetAnimation
		}
		if strings.TrimSpace(*edits) != "" {
			if err := json.Unmarshal([]byte(*edits), &options.Edits); err != nil {
				return fmt.Errorf("parse --edits: %w", err)
			}
		}
		options.Apply = *apply
		options.Overwrite = *overwrite
		value, err := spineops.PatchProjectTransform(options)
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
  spine233-agent-cli animations --file character.spine
  spine233-agent-cli rotate-timelines --file character.spine --animation attack
  spine233-agent-cli transform-timelines --file character.spine --animation attack
  spine233-agent-cli inspect   --file character.spine [--output-dir DIR]
  spine233-agent-cli query     --file character.json --pointer /animations/walk
  spine233-agent-cli analyze   --file character.json
  spine233-agent-cli validate  --file character.json
  spine233-agent-cli animate   --json character.json --source walk --target agent/walk --bone-motions JSON
  spine233-agent-cli animate-project --recipe agent-animation.json [--apply]
  spine233-agent-cli animate-project --file character.spine --animation attack --end-before idle --edits JSON
  spine233-agent-cli animate-project-rotate --recipe agent-animation.json [--apply]
  spine233-agent-cli animate-project-transform --recipe agent-animation.json [--apply]
  spine233-agent-cli patch     --file character.json --operations JSON [--output FILE --apply]
  spine233-agent-cli version

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
