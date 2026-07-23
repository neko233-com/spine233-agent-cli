// Package spineops provides bounded, agent-friendly Spine file operations.
package spineops

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	spineparser "github.com/neko233-com/spine233-file-parser"
)

// ProjectAnimationOptions controls direct .spine animation edits and optional
// record renaming. The operation is preview-first and never overwrites input.
type ProjectAnimationOptions struct {
	InputPath       string                           `json:"inputPath"`
	OutputPath      string                           `json:"outputPath,omitempty"`
	Animation       string                           `json:"animation"`
	TargetAnimation string                           `json:"targetAnimation,omitempty"`
	EndBefore       string                           `json:"endBefore,omitempty"`
	Edits           []spineparser.ProjectFloat32Edit `json:"edits"`
	Apply           bool                             `json:"apply,omitempty"`
	Overwrite       bool                             `json:"overwrite,omitempty"`
}

// ProjectAnimationResult reports a direct .spine preview or new project.
type ProjectAnimationResult struct {
	InputPath         string                                       `json:"inputPath"`
	OutputPath        string                                       `json:"outputPath,omitempty"`
	Applied           bool                                         `json:"applied"`
	InputSHA256       string                                       `json:"inputSha256"`
	OutputSHA256      string                                       `json:"outputSha256"`
	CompressedBytes   int                                          `json:"compressedBytes"`
	UncompressedBytes int                                          `json:"uncompressedBytes"`
	Patch             spineparser.ProjectAnimationFloatPatchReport `json:"patch"`
}

// PatchProjectAnimation previews or applies direct .spine float32 keyframe
// edits without invoking Spine Editor.
func PatchProjectAnimation(options ProjectAnimationOptions) (*ProjectAnimationResult, error) {
	absoluteInput, source, info, err := readFile(options.InputPath)
	if err != nil {
		return nil, err
	}
	document, err := spineparser.DeserializeProject(source, spineparser.InspectOptions{})
	if err != nil {
		return nil, err
	}
	patched, report, err := spineparser.PatchProjectAnimationFloat32(
		document,
		spineparser.ProjectAnimationFloatPatch{
			Animation:       options.Animation,
			TargetAnimation: options.TargetAnimation,
			EndBefore:       options.EndBefore,
			Edits:           options.Edits,
		},
	)
	if err != nil {
		return nil, err
	}
	encoded, err := spineparser.SerializeProject(patched, spineparser.ProjectSerializeOptions{})
	if err != nil {
		return nil, err
	}
	verified, err := spineparser.DeserializeProject(encoded, spineparser.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("verify serialized project: %w", err)
	}
	if !bytes.Equal(verified.Payload, patched.Payload) {
		return nil, errors.New("verify serialized project: payload mismatch")
	}

	inputHash := sha256.Sum256(source)
	outputHash := sha256.Sum256(encoded)
	result := &ProjectAnimationResult{
		InputPath:         absoluteInput,
		Applied:           false,
		InputSHA256:       hex.EncodeToString(inputHash[:]),
		OutputSHA256:      hex.EncodeToString(outputHash[:]),
		CompressedBytes:   len(encoded),
		UncompressedBytes: len(patched.Payload),
		Patch:             report,
	}
	if !options.Apply {
		return result, nil
	}
	if strings.TrimSpace(options.OutputPath) == "" {
		return nil, errors.New("outputPath is required when apply=true")
	}
	absoluteOutput, err := filepath.Abs(options.OutputPath)
	if err != nil {
		return nil, err
	}
	if samePath(absoluteInput, absoluteOutput) {
		return nil, errors.New("outputPath must differ from inputPath")
	}
	if !options.Overwrite {
		if _, err := os.Stat(absoluteOutput); err == nil {
			return nil, fmt.Errorf("outputPath already exists; set overwrite=true: %s", absoluteOutput)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := writeFileAtomic(absoluteOutput, encoded, info.Mode().Perm()); err != nil {
		return nil, err
	}
	result.OutputPath = absoluteOutput
	result.Applied = true
	return result, nil
}

// FileInfo is a lightweight file detection result.
type FileInfo struct {
	Path string               `json:"path"`
	Kind spineparser.FileKind `json:"kind"`
	Size int64                `json:"size"`
}

// ProjectAnimationList is a directly decoded .spine animation directory.
type ProjectAnimationList struct {
	Path         string                                 `json:"path"`
	SpineVersion string                                 `json:"spineVersion,omitempty"`
	Directory    *spineparser.ProjectAnimationDirectory `json:"directory"`
}

// ListProjectAnimations decodes top-level animation names and record boundaries
// without invoking Spine Editor.
func ListProjectAnimations(path string) (*ProjectAnimationList, error) {
	absolute, source, _, err := readFile(path)
	if err != nil {
		return nil, err
	}
	document, err := spineparser.DeserializeProject(source, spineparser.InspectOptions{})
	if err != nil {
		return nil, err
	}
	directory, err := spineparser.DiscoverProjectAnimations(document.Payload)
	if err != nil {
		return nil, err
	}
	return &ProjectAnimationList{
		Path:         absolute,
		SpineVersion: document.Inspection.SpineVersion,
		Directory:    directory,
	}, nil
}

// ProjectBoneList is a directly decoded .spine bone directory.
type ProjectBoneList struct {
	Path         string                            `json:"path"`
	SpineVersion string                            `json:"spineVersion,omitempty"`
	Directory    *spineparser.ProjectBoneDirectory `json:"directory"`
}

// ListProjectBones decodes bone names, offsets, and raw parent tokens without
// invoking Spine Editor.
func ListProjectBones(path string) (*ProjectBoneList, error) {
	absolute, source, _, err := readFile(path)
	if err != nil {
		return nil, err
	}
	document, err := spineparser.DeserializeProject(source, spineparser.InspectOptions{})
	if err != nil {
		return nil, err
	}
	directory, err := spineparser.DiscoverProjectBones(document.Payload)
	if err != nil {
		return nil, err
	}
	return &ProjectBoneList{
		Path:         absolute,
		SpineVersion: document.Inspection.SpineVersion,
		Directory:    directory,
	}, nil
}

// ProjectRotateTimelineList is a directly decoded .spine rotate directory.
type ProjectRotateTimelineList struct {
	Path         string                                      `json:"path"`
	SpineVersion string                                      `json:"spineVersion,omitempty"`
	Directory    *spineparser.ProjectRotateTimelineDirectory `json:"directory"`
}

// ListProjectRotateTimelines decodes one animation's rotate tracks and keys
// without invoking Spine Editor.
func ListProjectRotateTimelines(
	path string,
	animation string,
) (*ProjectRotateTimelineList, error) {
	absolute, source, _, err := readFile(path)
	if err != nil {
		return nil, err
	}
	document, err := spineparser.DeserializeProject(source, spineparser.InspectOptions{})
	if err != nil {
		return nil, err
	}
	directory, err := spineparser.DiscoverProjectRotateTimelines(
		document.Payload,
		animation,
	)
	if err != nil {
		return nil, err
	}
	return &ProjectRotateTimelineList{
		Path:         absolute,
		SpineVersion: document.Inspection.SpineVersion,
		Directory:    directory,
	}, nil
}

// ProjectRotateOptions controls semantic rotate-key edits. The operation is
// preview-first and never overwrites input.
type ProjectRotateOptions struct {
	InputPath       string                               `json:"inputPath"`
	OutputPath      string                               `json:"outputPath,omitempty"`
	Animation       string                               `json:"animation"`
	TargetAnimation string                               `json:"targetAnimation,omitempty"`
	Edits           []spineparser.ProjectRotateValueEdit `json:"edits"`
	Apply           bool                                 `json:"apply,omitempty"`
	Overwrite       bool                                 `json:"overwrite,omitempty"`
}

// ProjectRotateResult reports a semantic rotate preview or new project.
type ProjectRotateResult struct {
	InputPath         string                               `json:"inputPath"`
	OutputPath        string                               `json:"outputPath,omitempty"`
	Applied           bool                                 `json:"applied"`
	InputSHA256       string                               `json:"inputSha256"`
	OutputSHA256      string                               `json:"outputSha256"`
	CompressedBytes   int                                  `json:"compressedBytes"`
	UncompressedBytes int                                  `json:"uncompressedBytes"`
	Patch             spineparser.ProjectRotatePatchReport `json:"patch"`
}

// PatchProjectRotate previews or applies semantic rotate-key edits without
// invoking Spine Editor.
func PatchProjectRotate(options ProjectRotateOptions) (*ProjectRotateResult, error) {
	absoluteInput, source, info, err := readFile(options.InputPath)
	if err != nil {
		return nil, err
	}
	document, err := spineparser.DeserializeProject(source, spineparser.InspectOptions{})
	if err != nil {
		return nil, err
	}
	patched, report, err := spineparser.PatchProjectRotateValues(
		document,
		spineparser.ProjectRotatePatch{
			Animation:       options.Animation,
			TargetAnimation: options.TargetAnimation,
			Edits:           options.Edits,
		},
	)
	if err != nil {
		return nil, err
	}
	encoded, err := spineparser.SerializeProject(
		patched,
		spineparser.ProjectSerializeOptions{},
	)
	if err != nil {
		return nil, err
	}
	verified, err := spineparser.DeserializeProject(encoded, spineparser.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("verify serialized project: %w", err)
	}
	if !bytes.Equal(verified.Payload, patched.Payload) {
		return nil, errors.New("verify serialized project: payload mismatch")
	}

	inputHash := sha256.Sum256(source)
	outputHash := sha256.Sum256(encoded)
	result := &ProjectRotateResult{
		InputPath:         absoluteInput,
		Applied:           false,
		InputSHA256:       hex.EncodeToString(inputHash[:]),
		OutputSHA256:      hex.EncodeToString(outputHash[:]),
		CompressedBytes:   len(encoded),
		UncompressedBytes: len(patched.Payload),
		Patch:             report,
	}
	if !options.Apply {
		return result, nil
	}
	if strings.TrimSpace(options.OutputPath) == "" {
		return nil, errors.New("outputPath is required when apply=true")
	}
	absoluteOutput, err := filepath.Abs(options.OutputPath)
	if err != nil {
		return nil, err
	}
	if samePath(absoluteInput, absoluteOutput) {
		return nil, errors.New("outputPath must differ from inputPath")
	}
	if !options.Overwrite {
		if _, err := os.Stat(absoluteOutput); err == nil {
			return nil, fmt.Errorf(
				"outputPath already exists; set overwrite=true: %s",
				absoluteOutput,
			)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := writeFileAtomic(absoluteOutput, encoded, info.Mode().Perm()); err != nil {
		return nil, err
	}
	result.OutputPath = absoluteOutput
	result.Applied = true
	return result, nil
}

// ProjectTransformTimelineList is a directly decoded .spine bone transform
// timeline directory.
type ProjectTransformTimelineList struct {
	Path         string                                         `json:"path"`
	SpineVersion string                                         `json:"spineVersion,omitempty"`
	Directory    *spineparser.ProjectTransformTimelineDirectory `json:"directory"`
}

// ListProjectTransformTimelines decodes rotate, translate, scale, and shear
// tracks and keys without invoking Spine Editor.
func ListProjectTransformTimelines(
	path string,
	animation string,
) (*ProjectTransformTimelineList, error) {
	absolute, source, _, err := readFile(path)
	if err != nil {
		return nil, err
	}
	document, err := spineparser.DeserializeProject(source, spineparser.InspectOptions{})
	if err != nil {
		return nil, err
	}
	directory, err := spineparser.DiscoverProjectTransformTimelines(
		document.Payload,
		animation,
	)
	if err != nil {
		return nil, err
	}
	return &ProjectTransformTimelineList{
		Path:         absolute,
		SpineVersion: document.Inspection.SpineVersion,
		Directory:    directory,
	}, nil
}

// ProjectTransformOptions controls semantic bone transform edits. The
// operation is preview-first and never overwrites input.
type ProjectTransformOptions struct {
	InputPath       string                                  `json:"inputPath"`
	OutputPath      string                                  `json:"outputPath,omitempty"`
	Animation       string                                  `json:"animation"`
	TargetAnimation string                                  `json:"targetAnimation,omitempty"`
	Edits           []spineparser.ProjectTransformValueEdit `json:"edits"`
	Apply           bool                                    `json:"apply,omitempty"`
	Overwrite       bool                                    `json:"overwrite,omitempty"`
}

// ProjectTransformResult reports a semantic transform preview or new project.
type ProjectTransformResult struct {
	InputPath         string                                  `json:"inputPath"`
	OutputPath        string                                  `json:"outputPath,omitempty"`
	Applied           bool                                    `json:"applied"`
	InputSHA256       string                                  `json:"inputSha256"`
	OutputSHA256      string                                  `json:"outputSha256"`
	CompressedBytes   int                                     `json:"compressedBytes"`
	UncompressedBytes int                                     `json:"uncompressedBytes"`
	Patch             spineparser.ProjectTransformPatchReport `json:"patch"`
}

// PatchProjectTransform previews or applies semantic rotate, translate,
// scale, and shear key edits without invoking Spine Editor.
func PatchProjectTransform(
	options ProjectTransformOptions,
) (*ProjectTransformResult, error) {
	absoluteInput, source, info, err := readFile(options.InputPath)
	if err != nil {
		return nil, err
	}
	document, err := spineparser.DeserializeProject(source, spineparser.InspectOptions{})
	if err != nil {
		return nil, err
	}
	patched, report, err := spineparser.PatchProjectTransformValues(
		document,
		spineparser.ProjectTransformPatch{
			Animation:       options.Animation,
			TargetAnimation: options.TargetAnimation,
			Edits:           options.Edits,
		},
	)
	if err != nil {
		return nil, err
	}
	encoded, err := spineparser.SerializeProject(
		patched,
		spineparser.ProjectSerializeOptions{},
	)
	if err != nil {
		return nil, err
	}
	verified, err := spineparser.DeserializeProject(encoded, spineparser.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("verify serialized project: %w", err)
	}
	if !bytes.Equal(verified.Payload, patched.Payload) {
		return nil, errors.New("verify serialized project: payload mismatch")
	}

	inputHash := sha256.Sum256(source)
	outputHash := sha256.Sum256(encoded)
	result := &ProjectTransformResult{
		InputPath:         absoluteInput,
		Applied:           false,
		InputSHA256:       hex.EncodeToString(inputHash[:]),
		OutputSHA256:      hex.EncodeToString(outputHash[:]),
		CompressedBytes:   len(encoded),
		UncompressedBytes: len(patched.Payload),
		Patch:             report,
	}
	if !options.Apply {
		return result, nil
	}
	if strings.TrimSpace(options.OutputPath) == "" {
		return nil, errors.New("outputPath is required when apply=true")
	}
	absoluteOutput, err := filepath.Abs(options.OutputPath)
	if err != nil {
		return nil, err
	}
	if samePath(absoluteInput, absoluteOutput) {
		return nil, errors.New("outputPath must differ from inputPath")
	}
	if !options.Overwrite {
		if _, err := os.Stat(absoluteOutput); err == nil {
			return nil, fmt.Errorf(
				"outputPath already exists; set overwrite=true: %s",
				absoluteOutput,
			)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := writeFileAtomic(absoluteOutput, encoded, info.Mode().Perm()); err != nil {
		return nil, err
	}
	result.OutputPath = absoluteOutput
	result.Applied = true
	return result, nil
}

// ProjectTransformRewriteOptions controls declarative, fixed-topology
// animation rewriting. The operation is preview-first.
type ProjectTransformRewriteOptions struct {
	InputPath       string                                        `json:"inputPath"`
	OutputPath      string                                        `json:"outputPath,omitempty"`
	Animation       string                                        `json:"animation"`
	TargetAnimation string                                        `json:"targetAnimation,omitempty"`
	Timelines       []spineparser.ProjectTransformTimelineRewrite `json:"timelines"`
	Apply           bool                                          `json:"apply,omitempty"`
	Overwrite       bool                                          `json:"overwrite,omitempty"`
}

// ProjectTransformRecipe is a complete editable recipe generated from an
// existing .spine animation.
type ProjectTransformRecipe struct {
	SchemaVersion   string                                        `json:"schemaVersion"`
	InputPath       string                                        `json:"inputPath"`
	OutputPath      string                                        `json:"outputPath"`
	Animation       string                                        `json:"animation"`
	TargetAnimation string                                        `json:"targetAnimation"`
	Timelines       []spineparser.ProjectTransformTimelineRewrite `json:"timelines"`
}

// ProjectTransformRecipeBuildOptions controls recipe generation. Empty
// filters select every transform timeline.
type ProjectTransformRecipeBuildOptions struct {
	Path            string
	Animation       string
	TargetAnimation string
	OutputPath      string
	IncludeCurves   bool
	BoneReferences  []int
	TimelineTypes   []string
}

// BuildProjectTransformRecipe returns a complete fixed-topology recipe for
// Codex to edit. It does not write any files.
func BuildProjectTransformRecipe(
	path string,
	animation string,
	targetAnimation string,
	outputPath string,
	includeCurves bool,
) (*ProjectTransformRecipe, error) {
	return BuildProjectTransformRecipeWithOptions(ProjectTransformRecipeBuildOptions{
		Path:            path,
		Animation:       animation,
		TargetAnimation: targetAnimation,
		OutputPath:      outputPath,
		IncludeCurves:   includeCurves,
	})
}

// BuildProjectTransformRecipeWithOptions returns a full or filtered
// fixed-topology recipe. It does not write any files.
func BuildProjectTransformRecipeWithOptions(
	options ProjectTransformRecipeBuildOptions,
) (*ProjectTransformRecipe, error) {
	listed, err := ListProjectTransformTimelines(options.Path, options.Animation)
	if err != nil {
		return nil, err
	}
	boneReferences, err := normalizeBoneReferenceFilter(
		options.BoneReferences,
		listed.Directory.Timelines,
	)
	if err != nil {
		return nil, err
	}
	timelineTypes, err := normalizeTimelineTypeFilter(options.TimelineTypes)
	if err != nil {
		return nil, err
	}
	targetAnimation := options.TargetAnimation
	if strings.TrimSpace(targetAnimation) == "" {
		targetAnimation = options.Animation + "-agent"
	}
	outputPath := options.OutputPath
	if strings.TrimSpace(outputPath) == "" {
		extension := filepath.Ext(listed.Path)
		stem := strings.TrimSuffix(filepath.Base(listed.Path), extension)
		if strings.HasSuffix(strings.ToLower(stem), "-human") {
			stem = stem[:len(stem)-len("-human")]
		}
		outputPath = filepath.Join(filepath.Dir(listed.Path), stem+"-agent"+extension)
	} else {
		outputPath, err = filepath.Abs(outputPath)
		if err != nil {
			return nil, err
		}
	}
	timelines := make(
		[]spineparser.ProjectTransformTimelineRewrite,
		0,
		len(listed.Directory.Timelines),
	)
	matchedBoneReferences := make(map[int]struct{})
	matchedTimelineTypes := make(map[string]struct{})
	for _, timeline := range listed.Directory.Timelines {
		if len(boneReferences) != 0 {
			if _, selected := boneReferences[timeline.BoneReference]; !selected {
				continue
			}
		}
		if len(timelineTypes) != 0 {
			if _, selected := timelineTypes[timeline.Type]; !selected {
				continue
			}
		}
		keys := make(
			[]spineparser.ProjectTransformKeySpec,
			0,
			len(timeline.Keys),
		)
		for _, key := range timeline.Keys {
			spec := spineparser.ProjectTransformKeySpec{
				Frame:  key.Frame,
				Values: append([]float32(nil), key.Values...),
			}
			if options.IncludeCurves {
				spec.Curves = append([][4]float32(nil), key.Curves...)
			}
			keys = append(keys, spec)
		}
		timelines = append(timelines, spineparser.ProjectTransformTimelineRewrite{
			BoneReference: timeline.BoneReference,
			Timeline:      timeline.Type,
			Keys:          keys,
		})
		matchedBoneReferences[timeline.BoneReference] = struct{}{}
		matchedTimelineTypes[timeline.Type] = struct{}{}
	}
	if len(timelines) == 0 {
		return nil, errors.New("no transform timelines matched the requested filters")
	}
	for reference := range boneReferences {
		if _, matched := matchedBoneReferences[reference]; !matched {
			return nil, fmt.Errorf(
				"bone reference %d has no timeline matching the requested types",
				reference,
			)
		}
	}
	for timelineType := range timelineTypes {
		if _, matched := matchedTimelineTypes[timelineType]; !matched {
			return nil, fmt.Errorf(
				"timeline type %q has no match for the requested bones",
				timelineType,
			)
		}
	}
	return &ProjectTransformRecipe{
		SchemaVersion:   "spine233.transform-rewrite/v1",
		InputPath:       listed.Path,
		OutputPath:      outputPath,
		Animation:       options.Animation,
		TargetAnimation: targetAnimation,
		Timelines:       timelines,
	}, nil
}

func normalizeBoneReferenceFilter(
	requested []int,
	timelines []spineparser.ProjectTransformTimeline,
) (map[int]struct{}, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	available := make(map[int]struct{})
	for _, timeline := range timelines {
		available[timeline.BoneReference] = struct{}{}
	}
	selected := make(map[int]struct{}, len(requested))
	for _, reference := range requested {
		if reference < 0 {
			return nil, fmt.Errorf("bone reference must be non-negative: %d", reference)
		}
		if _, exists := available[reference]; !exists {
			return nil, fmt.Errorf(
				"bone reference %d has no transform timeline in animation",
				reference,
			)
		}
		selected[reference] = struct{}{}
	}
	return selected, nil
}

func normalizeTimelineTypeFilter(requested []string) (map[string]struct{}, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	selected := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		timelineType := strings.ToLower(strings.TrimSpace(raw))
		switch timelineType {
		case spineparser.ProjectTimelineRotate,
			spineparser.ProjectTimelineTranslate,
			spineparser.ProjectTimelineScale,
			spineparser.ProjectTimelineShear:
			selected[timelineType] = struct{}{}
		default:
			return nil, fmt.Errorf("unsupported transform timeline type %q", raw)
		}
	}
	return selected, nil
}

// RewriteProjectTransform previews or applies complete transform timeline
// declarations without invoking Spine Editor.
func RewriteProjectTransform(
	options ProjectTransformRewriteOptions,
) (*ProjectTransformResult, error) {
	absoluteInput, source, info, err := readFile(options.InputPath)
	if err != nil {
		return nil, err
	}
	document, err := spineparser.DeserializeProject(source, spineparser.InspectOptions{})
	if err != nil {
		return nil, err
	}
	patched, report, err := spineparser.RewriteProjectTransformTimelines(
		document,
		spineparser.ProjectTransformRewrite{
			Animation:       options.Animation,
			TargetAnimation: options.TargetAnimation,
			Timelines:       options.Timelines,
		},
	)
	if err != nil {
		return nil, err
	}
	encoded, err := spineparser.SerializeProject(
		patched,
		spineparser.ProjectSerializeOptions{},
	)
	if err != nil {
		return nil, err
	}
	verified, err := spineparser.DeserializeProject(encoded, spineparser.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("verify serialized project: %w", err)
	}
	if !bytes.Equal(verified.Payload, patched.Payload) {
		return nil, errors.New("verify serialized project: payload mismatch")
	}
	inputHash := sha256.Sum256(source)
	outputHash := sha256.Sum256(encoded)
	result := &ProjectTransformResult{
		InputPath:         absoluteInput,
		Applied:           false,
		InputSHA256:       hex.EncodeToString(inputHash[:]),
		OutputSHA256:      hex.EncodeToString(outputHash[:]),
		CompressedBytes:   len(encoded),
		UncompressedBytes: len(patched.Payload),
		Patch:             report,
	}
	if !options.Apply {
		return result, nil
	}
	if strings.TrimSpace(options.OutputPath) == "" {
		return nil, errors.New("outputPath is required when apply=true")
	}
	absoluteOutput, err := filepath.Abs(options.OutputPath)
	if err != nil {
		return nil, err
	}
	if samePath(absoluteInput, absoluteOutput) {
		return nil, errors.New("outputPath must differ from inputPath")
	}
	if !options.Overwrite {
		if _, err := os.Stat(absoluteOutput); err == nil {
			return nil, fmt.Errorf(
				"outputPath already exists; set overwrite=true: %s",
				absoluteOutput,
			)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := writeFileAtomic(absoluteOutput, encoded, info.Mode().Perm()); err != nil {
		return nil, err
	}
	result.OutputPath = absoluteOutput
	result.Applied = true
	return result, nil
}

// Detect identifies a local Spine file without invoking Spine Editor.
func Detect(path string) (*FileInfo, error) {
	absolute, source, info, err := readFile(path)
	if err != nil {
		return nil, err
	}
	return &FileInfo{Path: absolute, Kind: spineparser.Detect(source), Size: info.Size()}, nil
}

// Summary is a compact, token-efficient view of a Spine file.
type Summary struct {
	Path           string               `json:"path"`
	Kind           spineparser.FileKind `json:"kind"`
	SpineVersion   string               `json:"spineVersion,omitempty"`
	Hash           string               `json:"hash,omitempty"`
	Width          float64              `json:"width,omitempty"`
	Height         float64              `json:"height,omitempty"`
	Bones          []string             `json:"bones,omitempty"`
	Slots          []string             `json:"slots,omitempty"`
	Skins          []string             `json:"skins,omitempty"`
	Events         []string             `json:"events,omitempty"`
	Animations     []string             `json:"animations,omitempty"`
	ProjectStrings []string             `json:"projectStrings,omitempty"`
	Counts         map[string]int       `json:"counts,omitempty"`
	Truncated      map[string]bool      `json:"truncated,omitempty"`
}

const (
	maxSummaryNames  = 500
	defaultQuerySize = 1024 * 1024
)

// Summarize reads .spine metadata, .skel headers, or semantic Spine JSON.
// Private .spine semantics require Export; this method does not launch Spine.
func Summarize(path string) (*Summary, error) {
	absolute, source, _, err := readFile(path)
	if err != nil {
		return nil, err
	}
	kind := spineparser.Detect(source)
	switch kind {
	case spineparser.FileProject:
		inspection, err := spineparser.InspectProject(source, spineparser.InspectOptions{})
		if err != nil {
			return nil, err
		}
		result := &Summary{
			Path:         absolute,
			Kind:         kind,
			SpineVersion: inspection.SpineVersion,
			Counts:       map[string]int{"projectStrings": len(inspection.Strings)},
		}
		if document, decodeErr := spineparser.DeserializeProject(
			source,
			spineparser.InspectOptions{},
		); decodeErr == nil {
			if directory, discoverErr := spineparser.DiscoverProjectAnimations(
				document.Payload,
			); discoverErr == nil {
				for _, record := range directory.Records {
					result.Animations = append(result.Animations, record.Name)
				}
				result.Counts["animations"] = directory.Count
			}
			if bones, discoverErr := spineparser.DiscoverProjectBones(
				document.Payload,
			); discoverErr == nil {
				for _, record := range bones.Records {
					result.Bones = append(result.Bones, record.Name)
				}
				result.Counts["bones"] = bones.Count
			}
		}
		result.ProjectStrings, result.Truncated = boundedNames(inspection.Strings, "projectStrings", result.Truncated)
		return result, nil
	case spineparser.FileSkeletonBinary:
		inspection, err := spineparser.InspectSkeletonBinary(source)
		if err != nil {
			return nil, err
		}
		return &Summary{
			Path:         absolute,
			Kind:         kind,
			SpineVersion: inspection.SpineVersion,
			Hash:         inspection.Hash,
			Width:        float64(inspection.Width),
			Height:       float64(inspection.Height),
		}, nil
	case spineparser.FileSkeletonJSON:
		document, err := spineparser.ParseJSON(source)
		if err != nil {
			return nil, err
		}
		return summarizeJSON(absolute, document), nil
	default:
		return nil, fmt.Errorf("unsupported or invalid Spine file: %s", absolute)
	}
}

func summarizeJSON(path string, document *spineparser.SpineJSON) *Summary {
	result := &Summary{Path: path, Kind: spineparser.FileSkeletonJSON}
	if document.Skeleton != nil {
		result.SpineVersion = document.Skeleton.Spine
		result.Hash = document.Skeleton.Hash
		result.Width = document.Skeleton.Width
		result.Height = document.Skeleton.Height
	}
	bones := make([]string, 0, len(document.Bones))
	for _, bone := range document.Bones {
		bones = append(bones, bone.Name)
	}
	slots := make([]string, 0, len(document.Slots))
	for _, slot := range document.Slots {
		slots = append(slots, slot.Name)
	}
	animations := sortedKeys(document.Animations)
	events := sortedKeys(document.Events)
	skins := skinNames(document.Skins)
	result.Counts = map[string]int{
		"bones":      len(bones),
		"slots":      len(slots),
		"skins":      len(skins),
		"events":     len(events),
		"animations": len(animations),
	}
	result.Bones, result.Truncated = boundedNames(bones, "bones", result.Truncated)
	result.Slots, result.Truncated = boundedNames(slots, "slots", result.Truncated)
	result.Skins, result.Truncated = boundedNames(skins, "skins", result.Truncated)
	result.Events, result.Truncated = boundedNames(events, "events", result.Truncated)
	result.Animations, result.Truncated = boundedNames(animations, "animations", result.Truncated)
	return result
}

func boundedNames(values []string, key string, truncated map[string]bool) ([]string, map[string]bool) {
	if len(values) <= maxSummaryNames {
		return values, truncated
	}
	if truncated == nil {
		truncated = make(map[string]bool)
	}
	truncated[key] = true
	return values[:maxSummaryNames], truncated
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func skinNames(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil {
		return sortedKeys(object)
	}
	var array []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &array) == nil {
		names := make([]string, 0, len(array))
		for _, skin := range array {
			if skin.Name != "" {
				names = append(names, skin.Name)
			}
		}
		sort.Strings(names)
		return names
	}
	return nil
}

// InspectOptions controls diagnostic project inspection.
type InspectOptions struct {
	Path              string `json:"path"`
	OutputDirectory   string `json:"outputDirectory,omitempty"`
	OmitDecodedBinary bool   `json:"omitDecodedBinary,omitempty"`
}

// Inspect inspects a private .spine project and keeps diagnostic artifacts.
func Inspect(options InspectOptions) (*spineparser.InspectFileResult, error) {
	if strings.TrimSpace(options.Path) == "" {
		return nil, errors.New("path is required")
	}
	return spineparser.InspectFile(options.Path, spineparser.InspectFileOptions{
		OutputDirectory:   options.OutputDirectory,
		OmitDecodedBinary: options.OmitDecodedBinary,
	})
}

// QueryResult is a JSON Pointer query result.
type QueryResult struct {
	Path    string `json:"path"`
	Pointer string `json:"pointer"`
	Value   any    `json:"value"`
}

// Analyze returns a deep, version-tolerant Spine JSON feature inventory.
func Analyze(path string) (*spineparser.ProjectAnalysis, error) {
	_, source, _, err := readFile(path)
	if err != nil {
		return nil, err
	}
	return spineparser.AnalyzeJSON(source)
}

// Validate returns stable cross-version reference validation for Spine JSON.
func Validate(path string) (*spineparser.ValidationReport, error) {
	_, source, _, err := readFile(path)
	if err != nil {
		return nil, err
	}
	return spineparser.ValidateSemanticJSON(source)
}

// AnimationOptions controls preview-first animation generation.
type AnimationOptions struct {
	InputPath       string                   `json:"inputPath"`
	OutputPath      string                   `json:"outputPath,omitempty"`
	Source          string                   `json:"source"`
	Target          string                   `json:"target"`
	TimeScale       float64                  `json:"timeScale,omitempty"`
	BoneMotions     []spineparser.BoneMotion `json:"boneMotions,omitempty"`
	MarkerEvent     string                   `json:"markerEvent,omitempty"`
	ReplaceExisting bool                     `json:"replaceExisting,omitempty"`
	Apply           bool                     `json:"apply,omitempty"`
	Overwrite       bool                     `json:"overwrite,omitempty"`
}

// AnimationResult describes generated semantic data and optional output.
type AnimationResult struct {
	InputPath  string                            `json:"inputPath"`
	OutputPath string                            `json:"outputPath,omitempty"`
	Applied    bool                              `json:"applied"`
	Animation  *spineparser.CloneAnimationResult `json:"animation"`
	Analysis   *spineparser.ProjectAnalysis      `json:"analysis"`
	Validation *spineparser.ValidationReport     `json:"validation"`
}

// GenerateAnimation clones and edits one animation. It never overwrites input.
func GenerateAnimation(options AnimationOptions) (*AnimationResult, error) {
	absoluteInput, source, info, err := readFile(options.InputPath)
	if err != nil {
		return nil, err
	}
	encoded, animation, err := spineparser.CloneAnimation(source, spineparser.CloneAnimationOptions{
		Source:          options.Source,
		Target:          options.Target,
		TimeScale:       options.TimeScale,
		BoneMotions:     options.BoneMotions,
		MarkerEvent:     options.MarkerEvent,
		ReplaceExisting: options.ReplaceExisting,
		Indent:          "  ",
	})
	if err != nil {
		return nil, err
	}
	analysis, err := spineparser.AnalyzeJSON(encoded)
	if err != nil {
		return nil, err
	}
	validation, err := spineparser.ValidateSemanticJSON(encoded)
	if err != nil {
		return nil, err
	}
	result := &AnimationResult{
		InputPath:  absoluteInput,
		Applied:    false,
		Animation:  animation,
		Analysis:   analysis,
		Validation: validation,
	}
	if !options.Apply {
		return result, nil
	}
	if strings.TrimSpace(options.OutputPath) == "" {
		return nil, errors.New("outputPath is required when apply=true")
	}
	absoluteOutput, err := filepath.Abs(options.OutputPath)
	if err != nil {
		return nil, err
	}
	if samePath(absoluteInput, absoluteOutput) {
		return nil, errors.New("outputPath must differ from inputPath")
	}
	if !options.Overwrite {
		if _, err := os.Stat(absoluteOutput); err == nil {
			return nil, fmt.Errorf("outputPath already exists; set overwrite=true: %s", absoluteOutput)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := writeFileAtomic(absoluteOutput, encoded, info.Mode().Perm()); err != nil {
		return nil, err
	}
	result.OutputPath = absoluteOutput
	result.Applied = true
	return result, nil
}

// QueryJSON reads a bounded subtree using RFC 6901 JSON Pointer syntax.
func QueryJSON(path, pointer string, requestedMaxBytes ...int) (*QueryResult, error) {
	absolute, source, _, err := readFile(path)
	if err != nil {
		return nil, err
	}
	root, err := decodeAny(source)
	if err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	value, err := lookup(root, pointer)
	if err != nil {
		return nil, err
	}
	maxBytes := defaultQuerySize
	if len(requestedMaxBytes) > 0 {
		maxBytes = requestedMaxBytes[0]
	}
	if maxBytes < 1 || maxBytes > 16*1024*1024 {
		return nil, errors.New("maxBytes must be between 1 and 16777216")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(encoded) > maxBytes {
		return nil, fmt.Errorf("JSON Pointer result is %d bytes, exceeds maxBytes %d; query a narrower pointer", len(encoded), maxBytes)
	}
	return &QueryResult{Path: absolute, Pointer: pointer, Value: value}, nil
}

// PatchOperation is an RFC 6902-style add, replace, or remove operation.
type PatchOperation struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value,omitempty"`
}

// PatchOptions controls preview-first semantic JSON editing.
type PatchOptions struct {
	InputPath  string           `json:"inputPath"`
	OutputPath string           `json:"outputPath,omitempty"`
	Operations []PatchOperation `json:"operations"`
	Apply      bool             `json:"apply,omitempty"`
	Overwrite  bool             `json:"overwrite,omitempty"`
	Indent     string           `json:"indent,omitempty"`
}

// PatchChange records one before/after edit for agent review.
type PatchChange struct {
	Op     string `json:"op"`
	Path   string `json:"path"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

// PatchResult reports a preview or newly written JSON file.
type PatchResult struct {
	InputPath  string        `json:"inputPath"`
	OutputPath string        `json:"outputPath,omitempty"`
	Applied    bool          `json:"applied"`
	Changes    []PatchChange `json:"changes"`
	Summary    *Summary      `json:"summary"`
}

// PatchJSON previews or applies JSON patches. It never overwrites its input.
func PatchJSON(options PatchOptions) (*PatchResult, error) {
	if len(options.Operations) == 0 {
		return nil, errors.New("at least one patch operation is required")
	}
	absoluteInput, source, info, err := readFile(options.InputPath)
	if err != nil {
		return nil, err
	}
	root, err := decodeAny(source)
	if err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	changes := make([]PatchChange, 0, len(options.Operations))
	for index, operation := range options.Operations {
		var change PatchChange
		root, change, err = applyOperation(root, operation)
		if err != nil {
			return nil, fmt.Errorf("operation %d: %w", index, err)
		}
		changes = append(changes, change)
	}
	indent := options.Indent
	if indent == "" {
		indent = "  "
	}
	encoded, err := json.MarshalIndent(root, "", indent)
	if err != nil {
		return nil, err
	}
	encoded = append(encoded, '\n')
	document, err := spineparser.ParseJSON(encoded)
	if err != nil {
		return nil, fmt.Errorf("patched document is not valid Spine JSON: %w", err)
	}
	result := &PatchResult{
		InputPath: absoluteInput,
		Applied:   false,
		Changes:   changes,
		Summary:   summarizeJSON(absoluteInput, document),
	}
	if !options.Apply {
		return result, nil
	}
	if strings.TrimSpace(options.OutputPath) == "" {
		return nil, errors.New("outputPath is required when apply=true")
	}
	absoluteOutput, err := filepath.Abs(options.OutputPath)
	if err != nil {
		return nil, err
	}
	if samePath(absoluteInput, absoluteOutput) {
		return nil, errors.New("outputPath must differ from inputPath")
	}
	if !options.Overwrite {
		if _, err := os.Stat(absoluteOutput); err == nil {
			return nil, fmt.Errorf("outputPath already exists; set overwrite=true: %s", absoluteOutput)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := writeFileAtomic(absoluteOutput, encoded, info.Mode().Perm()); err != nil {
		return nil, err
	}
	result.OutputPath = absoluteOutput
	result.Applied = true
	result.Summary.Path = absoluteOutput
	return result, nil
}

func decodeAny(source []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func lookup(root any, pointer string) (any, error) {
	tokens, err := pointerTokens(pointer)
	if err != nil {
		return nil, err
	}
	current := root
	for _, token := range tokens {
		switch value := current.(type) {
		case map[string]any:
			next, ok := value[token]
			if !ok {
				return nil, fmt.Errorf("JSON Pointer key not found: %s", token)
			}
			current = next
		case []any:
			index, err := arrayIndex(token, len(value), false)
			if err != nil {
				return nil, err
			}
			current = value[index]
		default:
			return nil, fmt.Errorf("JSON Pointer traverses scalar at %q", token)
		}
	}
	return current, nil
}

func applyOperation(root any, operation PatchOperation) (any, PatchChange, error) {
	op := strings.ToLower(strings.TrimSpace(operation.Op))
	if op != "add" && op != "replace" && op != "remove" {
		return root, PatchChange{}, fmt.Errorf("unsupported op %q", operation.Op)
	}
	tokens, err := pointerTokens(operation.Path)
	if err != nil {
		return root, PatchChange{}, err
	}
	var after any
	if op != "remove" {
		if len(operation.Value) == 0 {
			return root, PatchChange{}, fmt.Errorf("%s requires value", op)
		}
		after, err = decodeAny(operation.Value)
		if err != nil {
			return root, PatchChange{}, fmt.Errorf("decode value: %w", err)
		}
	}
	change := PatchChange{Op: op, Path: operation.Path, After: after}
	if len(tokens) == 0 {
		if op == "remove" {
			return root, PatchChange{}, errors.New("cannot remove document root")
		}
		change.Before = root
		return after, change, nil
	}
	parentPointer := ""
	if len(tokens) > 1 {
		encoded := make([]string, len(tokens)-1)
		for index, token := range tokens[:len(tokens)-1] {
			encoded[index] = strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
		}
		parentPointer = "/" + strings.Join(encoded, "/")
	}
	parent, err := lookup(root, parentPointer)
	if err != nil {
		return root, PatchChange{}, err
	}
	key := tokens[len(tokens)-1]
	switch value := parent.(type) {
	case map[string]any:
		before, exists := value[key]
		if op != "add" && !exists {
			return root, PatchChange{}, fmt.Errorf("JSON Pointer key not found: %s", key)
		}
		change.Before = before
		if op == "remove" {
			delete(value, key)
			change.After = nil
		} else {
			value[key] = after
		}
	case []any:
		index, indexErr := arrayIndex(key, len(value), op == "add")
		if indexErr != nil {
			return root, PatchChange{}, indexErr
		}
		switch op {
		case "add":
			if index == len(value) {
				value = append(value, after)
			} else {
				value = append(value[:index], append([]any{after}, value[index:]...)...)
			}
		case "replace":
			change.Before = value[index]
			value[index] = after
		case "remove":
			change.Before = value[index]
			change.After = nil
			value = append(value[:index], value[index+1:]...)
		}
		root, err = replaceContainer(root, tokens[:len(tokens)-1], value)
		if err != nil {
			return root, PatchChange{}, err
		}
	default:
		return root, PatchChange{}, fmt.Errorf("patch parent is scalar: %s", parentPointer)
	}
	return root, change, nil
}

func replaceContainer(root any, tokens []string, replacement any) (any, error) {
	if len(tokens) == 0 {
		return replacement, nil
	}
	parentPointer := ""
	if len(tokens) > 1 {
		encoded := make([]string, len(tokens)-1)
		for index, token := range tokens[:len(tokens)-1] {
			encoded[index] = strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
		}
		parentPointer = "/" + strings.Join(encoded, "/")
	}
	parent, err := lookup(root, parentPointer)
	if err != nil {
		return root, err
	}
	key := tokens[len(tokens)-1]
	switch value := parent.(type) {
	case map[string]any:
		value[key] = replacement
	case []any:
		index, err := arrayIndex(key, len(value), false)
		if err != nil {
			return root, err
		}
		value[index] = replacement
	default:
		return root, errors.New("invalid JSON container")
	}
	return root, nil
}

func pointerTokens(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, errors.New("JSON Pointer must be empty or start with /")
	}
	raw := strings.Split(pointer[1:], "/")
	tokens := make([]string, len(raw))
	for index, token := range raw {
		var output strings.Builder
		for offset := 0; offset < len(token); offset++ {
			if token[offset] != '~' {
				output.WriteByte(token[offset])
				continue
			}
			if offset+1 >= len(token) || (token[offset+1] != '0' && token[offset+1] != '1') {
				return nil, fmt.Errorf("invalid JSON Pointer escape in %q", token)
			}
			offset++
			if token[offset] == '0' {
				output.WriteByte('~')
			} else {
				output.WriteByte('/')
			}
		}
		tokens[index] = output.String()
	}
	return tokens, nil
}

func arrayIndex(token string, length int, allowEnd bool) (int, error) {
	if token == "-" {
		if allowEnd {
			return length, nil
		}
		return 0, errors.New("'-' array index is only valid for add")
	}
	index, err := strconv.Atoi(token)
	if err != nil || index < 0 || index >= length+boolInt(allowEnd) {
		return 0, fmt.Errorf("invalid array index %q for length %d", token, length)
	}
	return index, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func readFile(path string) (string, []byte, os.FileInfo, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil, nil, errors.New("path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, nil, fmt.Errorf("path is not a regular file: %s", absolute)
	}
	source, err := os.ReadFile(absolute)
	if err != nil {
		return "", nil, nil, err
	}
	return absolute, source, info, nil
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
