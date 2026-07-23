package spineops

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	spineparser "github.com/neko233-com/spine233-file-parser"
)

const testJSON = `{
  "skeleton": {"spine": "4.2.0", "width": 100, "height": 200},
  "bones": [{"name": "root"}, {"name": "arm", "parent": "root"}],
  "slots": [{"name": "hand", "bone": "arm"}],
  "skins": [{"name": "default"}, {"name": "winter"}],
  "events": {"step": {}},
  "animations": {"walk": {}, "idle": {}}
}`

func writeTestJSON(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hero.json")
	if err := os.WriteFile(path, []byte(testJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDetectAndSummarizeJSON(t *testing.T) {
	path := writeTestJSON(t)
	info, err := Detect(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != "skeleton-json" {
		t.Fatalf("kind = %q", info.Kind)
	}
	summary, err := Summarize(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SpineVersion != "4.2.0" || summary.Counts["animations"] != 2 || len(summary.Skins) != 2 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestQueryJSONPointer(t *testing.T) {
	result, err := QueryJSON(writeTestJSON(t), "/bones/1/name")
	if err != nil {
		t.Fatal(err)
	}
	if result.Value != "arm" {
		t.Fatalf("value = %#v", result.Value)
	}
}

func TestPatchPreviewAndApply(t *testing.T) {
	input := writeTestJSON(t)
	output := filepath.Join(t.TempDir(), "patched.json")
	operations := []PatchOperation{
		{Op: "replace", Path: "/bones/1/name", Value: json.RawMessage(`"wing"`)},
		{Op: "add", Path: "/animations/fly", Value: json.RawMessage(`{}`)},
		{Op: "remove", Path: "/animations/walk"},
	}
	preview, err := PatchJSON(PatchOptions{InputPath: input, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || len(preview.Changes) != 3 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preview wrote output: %v", err)
	}
	applied, err := PatchJSON(PatchOptions{
		InputPath: input, OutputPath: output, Operations: operations, Apply: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || applied.Summary.Counts["animations"] != 2 {
		t.Fatalf("unexpected apply result: %#v", applied)
	}
	query, err := QueryJSON(output, "/bones/1/name")
	if err != nil {
		t.Fatal(err)
	}
	if query.Value != "wing" {
		t.Fatalf("patched value = %#v", query.Value)
	}
}

func TestPatchNeverOverwritesInput(t *testing.T) {
	input := writeTestJSON(t)
	_, err := PatchJSON(PatchOptions{
		InputPath: input, OutputPath: input, Apply: true,
		Operations: []PatchOperation{{Op: "add", Path: "/animations/new", Value: json.RawMessage(`{}`)}},
	})
	if err == nil {
		t.Fatal("expected same-path error")
	}
}

func TestInvalidPointerEscape(t *testing.T) {
	_, err := QueryJSON(writeTestJSON(t), "/bad~2key")
	if err == nil {
		t.Fatal("expected invalid pointer error")
	}
}

func TestQueryJSONSizeLimit(t *testing.T) {
	_, err := QueryJSON(writeTestJSON(t), "/bones", 2)
	if err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestPatchProjectAnimationOfficialHero(t *testing.T) {
	input := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	output := filepath.Join(t.TempDir(), "hero-agent.spine")
	options := ProjectAnimationOptions{
		InputPath:  input,
		OutputPath: output,
		Animation:  "attack",
		Edits: []spineparser.ProjectFloat32Edit{
			{From: 13.22, To: 24, ExpectedMatches: 1},
		},
	}
	preview, err := PatchProjectAnimation(options)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || preview.OutputSHA256 == preview.InputSHA256 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preview wrote output: %v", err)
	}

	options.Apply = true
	applied, err := PatchProjectAnimation(options)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || len(applied.Patch.Changes) != 1 {
		t.Fatalf("unexpected apply result: %#v", applied)
	}
	inputBytes, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	outputBytes, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	before, err := spineparser.DeserializeProject(inputBytes, spineparser.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	after, err := spineparser.DeserializeProject(outputBytes, spineparser.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Payload) != len(after.Payload) {
		t.Fatalf("payload length changed: %d -> %d", len(before.Payload), len(after.Payload))
	}
	offset := applied.Patch.Changes[0].Offsets[0]
	for index := range before.Payload {
		if before.Payload[index] != after.Payload[index] &&
			(index < offset || index >= offset+4) {
			t.Fatalf("unexpected payload change at %d", index)
		}
	}
}

func TestListProjectAnimationsOfficialHero(t *testing.T) {
	human, err := ListProjectAnimations(
		filepath.Join("..", "..", "demo", "hero", "hero-human.spine"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if human.Directory.Count != 12 ||
		human.Directory.Records[0].Name != "attack" ||
		human.Directory.Records[11].Name != "walk" {
		t.Fatalf("human animations = %#v", human.Directory)
	}
	agent, err := ListProjectAnimations(
		filepath.Join("..", "..", "demo", "hero", "hero-agent.spine"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Directory.Count != 12 ||
		agent.Directory.Records[0].Name != "attack-agent" {
		t.Fatalf("agent animations = %#v", agent.Directory)
	}
}

func TestDeleteLastProjectAnimationPreviewAndDefaultApply(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	input := filepath.Join(directory, "hero-human.spine")
	if err := os.WriteFile(input, source, 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "hero-agent.spine")
	options := ProjectAnimationDeleteOptions{
		InputPath: input,
		Animation: "walk",
	}
	preview, err := DeleteLastProjectAnimation(options)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied ||
		preview.OutputPath != "" ||
		preview.InputSHA256 == preview.OutputSHA256 ||
		preview.Deletion.PreviousCount != 12 ||
		preview.Deletion.Count != 11 ||
		preview.Directory.Count != 11 ||
		preview.Directory.Records[10].Name != "run-from fall" {
		t.Fatalf("preview = %#v", preview)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preview wrote default output: %v", err)
	}

	options.Apply = true
	applied, err := DeleteLastProjectAnimation(options)
	if err != nil {
		t.Fatal(err)
	}
	absoluteOutput, err := filepath.Abs(output)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied ||
		applied.OutputPath != absoluteOutput ||
		applied.Directory.Count != 11 {
		t.Fatalf("applied = %#v", applied)
	}
	outputSource, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := spineparser.DeserializeProject(
		outputSource,
		spineparser.InspectOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	reloadedDirectory, err := spineparser.DiscoverProjectAnimations(
		reloaded.Payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedDirectory.Count != 11 ||
		reloadedDirectory.Records[10].Name != "run-from fall" {
		t.Fatalf("reloaded directory = %#v", reloadedDirectory)
	}
	unchanged, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, source) {
		t.Fatal("input project was overwritten")
	}
}

func TestDeleteLastProjectAnimationFailsClosed(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "hero-human.spine")
	if err := os.WriteFile(input, source, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DeleteLastProjectAnimation(ProjectAnimationDeleteOptions{
		InputPath: input,
		Animation: "attack",
	}); err == nil || !strings.Contains(err.Error(), "not the final") {
		t.Fatalf("non-terminal deletion err = %v", err)
	}
	if _, err := DeleteLastProjectAnimation(ProjectAnimationDeleteOptions{
		InputPath:  input,
		OutputPath: input,
		Animation:  "walk",
		Apply:      true,
		Overwrite:  true,
	}); err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("input overwrite err = %v", err)
	}
	unchanged, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, source) {
		t.Fatal("rejected deletion changed input")
	}

	document, err := spineparser.DeserializeProject(
		source,
		spineparser.InspectOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	for {
		animations, err := spineparser.DiscoverProjectAnimations(document.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if animations.Count == 1 {
			break
		}
		final := animations.Records[len(animations.Records)-1]
		document, _, err = spineparser.DeleteLastProjectAnimation(
			document,
			final.Name,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	onlyDirectory, err := spineparser.DiscoverProjectAnimations(document.Payload)
	if err != nil {
		t.Fatal(err)
	}
	onlySource, err := spineparser.SerializeProject(
		document,
		spineparser.ProjectSerializeOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	onlyPath := filepath.Join(t.TempDir(), "only.spine")
	if err := os.WriteFile(onlyPath, onlySource, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DeleteLastProjectAnimation(ProjectAnimationDeleteOptions{
		InputPath: onlyPath,
		Animation: onlyDirectory.Records[0].Name,
	}); err == nil || !strings.Contains(err.Error(), "only project animation") {
		t.Fatalf("only-animation deletion err = %v", err)
	}
}

func TestListProjectBonesOfficialDemos(t *testing.T) {
	tests := []struct {
		project string
		count   int
		names   []string
	}{
		{"alien", 28, []string{"root", "body", "metaljaw", "splat"}},
		{"hero", 44, []string{"root", "body", "head", "chain1"}},
		{"raptor", 76, []string{"root", "front-arm", "head-control", "leg-control"}},
	}
	for _, test := range tests {
		t.Run(test.project, func(t *testing.T) {
			path := filepath.Join(
				"..",
				"..",
				"demo",
				test.project,
				test.project+"-human.spine",
			)
			listed, err := ListProjectBones(path)
			if err != nil {
				t.Fatal(err)
			}
			if listed.Directory.Count != test.count {
				t.Fatalf("bone count = %d, want %d", listed.Directory.Count, test.count)
			}
			names := make(map[string]bool, listed.Directory.Count)
			for _, record := range listed.Directory.Records {
				names[record.Name] = true
			}
			for _, name := range test.names {
				if !names[name] {
					t.Fatalf("bone %q not found", name)
				}
			}
		})
	}
}

func TestProjectRotateOfficialHero(t *testing.T) {
	input := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	listed, err := ListProjectRotateTimelines(input, "attack")
	if err != nil {
		t.Fatal(err)
	}
	var body *spineparser.ProjectRotateTimeline
	for index := range listed.Directory.Timelines {
		if listed.Directory.Timelines[index].BoneReference == 6 {
			body = &listed.Directory.Timelines[index]
			break
		}
	}
	if body == nil || len(body.Keys) != 6 ||
		body.Keys[1].Frame != 2 || body.Keys[1].Value != float32(13.22) {
		t.Fatalf("body rotate timeline = %#v", body)
	}

	output := filepath.Join(t.TempDir(), "hero-agent.spine")
	options := ProjectRotateOptions{
		InputPath:       input,
		OutputPath:      output,
		Animation:       "attack",
		TargetAnimation: "attack-agent",
		Edits: []spineparser.ProjectRotateValueEdit{
			{BoneReference: 6, KeyIndex: 1, From: 13.22, To: 24},
		},
	}
	preview, err := PatchProjectRotate(options)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || len(preview.Patch.Changes) != 1 {
		t.Fatalf("preview = %#v", preview)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preview wrote output: %v", err)
	}
	options.Apply = true
	applied, err := PatchProjectRotate(options)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied {
		t.Fatalf("applied = %#v", applied)
	}
	renamed, err := ListProjectRotateTimelines(output, "attack-agent")
	if err != nil {
		t.Fatal(err)
	}
	for _, timeline := range renamed.Directory.Timelines {
		if timeline.BoneReference == 6 && timeline.Keys[1].Value == 24 {
			return
		}
	}
	t.Fatal("renamed semantic rotate value was not persisted")
}

func TestAgentDemoRotateProjects(t *testing.T) {
	tests := []struct {
		project    string
		source     string
		target     string
		boneRef    int
		keyIndex   int
		humanValue float32
		agentValue float32
	}{
		{"alien", "death", "death-agent", 9, 2, -10.509285, -18},
		{"hero", "attack", "attack-agent", 6, 1, 13.22, 24},
		{"raptor", "gun-grab", "gun-grab-agent", 40, 1, -32, -48},
	}
	for _, test := range tests {
		t.Run(test.project, func(t *testing.T) {
			directory := filepath.Join("..", "..", "demo", test.project)
			human, err := ListProjectRotateTimelines(
				filepath.Join(directory, test.project+"-human.spine"),
				test.source,
			)
			if err != nil {
				t.Fatal(err)
			}
			agent, err := ListProjectRotateTimelines(
				filepath.Join(directory, test.project+"-agent.spine"),
				test.target,
			)
			if err != nil {
				t.Fatal(err)
			}
			humanValue, ok := rotateValue(
				human.Directory,
				test.boneRef,
				test.keyIndex,
			)
			if !ok || humanValue != test.humanValue {
				t.Fatalf("human value = %v, %v", humanValue, ok)
			}
			agentValue, ok := rotateValue(
				agent.Directory,
				test.boneRef,
				test.keyIndex,
			)
			if !ok || agentValue != test.agentValue {
				t.Fatalf("agent value = %v, %v", agentValue, ok)
			}
		})
	}
}

func TestProjectTransformOfficialHero(t *testing.T) {
	input := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	listed, err := ListProjectTransformTimelines(input, "attack")
	if err != nil {
		t.Fatal(err)
	}
	var translate *spineparser.ProjectTransformTimeline
	for index := range listed.Directory.Timelines {
		timeline := &listed.Directory.Timelines[index]
		if timeline.BoneReference == 6 &&
			timeline.Type == spineparser.ProjectTimelineTranslate {
			translate = timeline
			break
		}
	}
	if translate == nil || len(translate.Keys) != 4 ||
		translate.Keys[1].Frame != 4 ||
		translate.Keys[1].Values[0] != float32(4.86) ||
		translate.Keys[1].Values[1] != float32(-0.24) {
		t.Fatalf("translate timeline = %#v", translate)
	}
	preview, err := PatchProjectTransform(ProjectTransformOptions{
		InputPath:       input,
		OutputPath:      filepath.Join(t.TempDir(), "hero-agent.spine"),
		Animation:       "attack",
		TargetAnimation: "attack-agent",
		Edits: []spineparser.ProjectTransformValueEdit{
			{
				BoneReference: 6,
				Timeline:      spineparser.ProjectTimelineTranslate,
				KeyIndex:      1,
				Channel:       "x",
				From:          4.86,
				To:            8,
			},
			{
				BoneReference: 6,
				Timeline:      spineparser.ProjectTimelineTranslate,
				KeyIndex:      1,
				Channel:       "frame",
				From:          4,
				To:            5,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || len(preview.Patch.Changes) != 2 ||
		preview.Patch.Changes[0].Timeline != spineparser.ProjectTimelineTranslate {
		t.Fatalf("preview = %#v", preview)
	}
}

func TestOfficialDemoTransformCoverage(t *testing.T) {
	tests := []struct {
		project   string
		timelines map[string]int
		keys      map[string]int
	}{
		{
			project: "alien",
			timelines: map[string]int{
				"rotate": 62, "translate": 25, "scale": 15, "shear": 1,
			},
			keys: map[string]int{
				"rotate": 407, "translate": 179, "scale": 92, "shear": 3,
			},
		},
		{
			project: "hero",
			timelines: map[string]int{
				"rotate": 159, "translate": 116, "scale": 5,
			},
			keys: map[string]int{
				"rotate": 742, "translate": 574, "scale": 15,
			},
		},
		{
			project: "raptor",
			timelines: map[string]int{
				"rotate": 161, "translate": 81, "scale": 14, "shear": 2,
			},
			keys: map[string]int{
				"rotate": 1217, "translate": 492, "scale": 72, "shear": 12,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.project, func(t *testing.T) {
			path := filepath.Join(
				"..",
				"..",
				"demo",
				test.project,
				test.project+"-human.spine",
			)
			animations, err := ListProjectAnimations(path)
			if err != nil {
				t.Fatal(err)
			}
			timelineCounts := make(map[string]int)
			keyCounts := make(map[string]int)
			for _, animation := range animations.Directory.Records {
				directory, err := ListProjectTransformTimelines(
					path,
					animation.Name,
				)
				if err != nil {
					t.Fatal(err)
				}
				for _, timeline := range directory.Directory.Timelines {
					timelineCounts[timeline.Type]++
					keyCounts[timeline.Type] += len(timeline.Keys)
				}
			}
			if !mapsEqual(timelineCounts, test.timelines) {
				t.Fatalf("timeline counts = %#v, want %#v", timelineCounts, test.timelines)
			}
			if !mapsEqual(keyCounts, test.keys) {
				t.Fatalf("key counts = %#v, want %#v", keyCounts, test.keys)
			}
		})
	}
}

func TestRewriteProjectTransformOfficialHero(t *testing.T) {
	input := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	output := filepath.Join(t.TempDir(), "hero-agent.spine")
	options := ProjectTransformRewriteOptions{
		InputPath:       input,
		OutputPath:      output,
		Animation:       "attack",
		TargetAnimation: "attack-agent",
		Timelines: []spineparser.ProjectTransformTimelineRewrite{
			{
				BoneReference: 6,
				Timeline:      spineparser.ProjectTimelineTranslate,
				Keys: []spineparser.ProjectTransformKeySpec{
					{Frame: 0, Values: []float32{-0.77, -1.89}},
					{Frame: 5, Values: []float32{8, -0.24}},
					{Frame: 6, Values: []float32{8.05, -2.44}},
					{Frame: 12, Values: []float32{-0.77, -1.89}},
				},
			},
		},
	}
	preview, err := RewriteProjectTransform(options)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || len(preview.Patch.Changes) != 2 {
		t.Fatalf("preview = %#v", preview)
	}
	options.Apply = true
	applied, err := RewriteProjectTransform(options)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied {
		t.Fatalf("applied = %#v", applied)
	}
	rewritten, err := ListProjectTransformTimelines(output, "attack-agent")
	if err != nil {
		t.Fatal(err)
	}
	for _, timeline := range rewritten.Directory.Timelines {
		if timeline.BoneReference == 6 &&
			timeline.Type == spineparser.ProjectTimelineTranslate {
			if timeline.Keys[1].Frame != 5 ||
				timeline.Keys[1].Values[0] != 8 {
				t.Fatalf("timeline = %#v", timeline)
			}
			return
		}
	}
	t.Fatal("rewritten translate timeline not found")
}

func TestBuildProjectTransformRecipeOfficialHero(t *testing.T) {
	input := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	recipe, err := BuildProjectTransformRecipe(
		input,
		"attack",
		"",
		"",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if recipe.SchemaVersion != "spine233.transform-rewrite/v1" ||
		recipe.TargetAnimation != "attack-agent" ||
		filepath.Base(recipe.OutputPath) != "hero-agent.spine" ||
		len(recipe.Timelines) != 25 {
		t.Fatalf("recipe = %#v", recipe)
	}
	for _, timeline := range recipe.Timelines {
		for _, key := range timeline.Keys {
			if len(key.Curves) != 0 {
				t.Fatal("curves included without includeCurves")
			}
		}
	}
	withCurves, err := BuildProjectTransformRecipe(
		input,
		"attack",
		"",
		"",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(withCurves.Timelines[0].Keys[0].Curves) == 0 {
		t.Fatal("includeCurves did not include curve controls")
	}
}

func TestBuildProjectTransformRecipeFiltersOfficialHero(t *testing.T) {
	input := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	recipe, err := BuildProjectTransformRecipeWithOptions(
		ProjectTransformRecipeBuildOptions{
			Path:           input,
			Animation:      "attack",
			BoneReferences: []int{6},
			TimelineTypes:  []string{" TRANSLATE "},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipe.Timelines) == 0 {
		t.Fatal("filtered recipe has no timelines")
	}
	for _, timeline := range recipe.Timelines {
		if timeline.BoneReference != 6 ||
			timeline.Timeline != spineparser.ProjectTimelineTranslate {
			t.Fatalf("unexpected filtered timeline: %#v", timeline)
		}
	}
	if _, err := BuildProjectTransformRecipeWithOptions(
		ProjectTransformRecipeBuildOptions{
			Path:          input,
			Animation:     "attack",
			TimelineTypes: []string{"color"},
		},
	); err == nil {
		t.Fatal("unsupported timeline filter accepted")
	}
	if _, err := BuildProjectTransformRecipeWithOptions(
		ProjectTransformRecipeBuildOptions{
			Path:           input,
			Animation:      "attack",
			BoneReferences: []int{999999},
		},
	); err == nil {
		t.Fatal("unknown bone reference accepted")
	}
}

func TestCompareProjectTransformAnimationsDemos(t *testing.T) {
	tests := []struct {
		role            string
		sourceAnimation string
		targetAnimation string
		changes         int
	}{
		{role: "alien", sourceAnimation: "death", targetAnimation: "death-agent", changes: 8},
		{role: "hero", sourceAnimation: "attack", targetAnimation: "attack-agent", changes: 4},
		{role: "raptor", sourceAnimation: "gun-grab", targetAnimation: "gun-grab-agent", changes: 4},
	}
	for _, test := range tests {
		t.Run(test.role, func(t *testing.T) {
			directory := filepath.Join("..", "..", "demo", test.role)
			comparison, err := CompareProjectTransformAnimations(
				ProjectTransformComparisonOptions{
					SourcePath: filepath.Join(
						directory,
						test.role+"-human.spine",
					),
					SourceAnimation: test.sourceAnimation,
					TargetPath: filepath.Join(
						directory,
						test.role+"-agent.spine",
					),
					TargetAnimation: test.targetAnimation,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if !comparison.TransformReady ||
				comparison.AgentReady ||
				comparison.CompleteAnimationVerified ||
				len(comparison.CapabilityGaps) == 0 ||
				!comparison.AgentNameValid ||
				!comparison.Compatible ||
				!comparison.SemanticChanged ||
				comparison.TotalChanges != test.changes {
				t.Fatalf("comparison = %#v", comparison)
			}
		})
	}
}

func TestProgramProjectTransformOfficialHero(t *testing.T) {
	input := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	output := filepath.Join(t.TempDir(), "hero-agent.spine")
	programmed, err := ProgramProjectTransform(ProjectTransformProgramOptions{
		InputPath:  input,
		OutputPath: output,
		Animation:  "attack",
		Operations: []ProjectTransformProgramOperation{
			{
				BoneReferences:  []int{6},
				Timeline:        "rotate",
				Channel:         "value",
				KeyIndices:      []int{1},
				Mode:            "add",
				Operand:         10,
				ExpectedMatches: 1,
			},
		},
		Apply: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !programmed.Result.Applied ||
		programmed.Result.Patch.TargetAnimation != "attack-agent" ||
		len(programmed.ExpandedEdits) != 1 ||
		programmed.ExpandedEdits[0].To !=
			programmed.ExpandedEdits[0].From+10 {
		t.Fatalf("programmed = %#v", programmed)
	}
	comparison, err := CompareProjectTransformAnimations(
		ProjectTransformComparisonOptions{
			SourcePath:      input,
			SourceAnimation: "attack",
			TargetPath:      output,
			TargetAnimation: "attack-agent",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !comparison.TransformReady ||
		comparison.AgentReady ||
		comparison.TotalChanges != 1 {
		t.Fatalf("comparison = %#v", comparison)
	}
}

func TestProgramProjectTransformRejectsMatchDrift(t *testing.T) {
	input := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	_, err := ProgramProjectTransform(ProjectTransformProgramOptions{
		InputPath: input,
		Animation: "attack",
		Operations: []ProjectTransformProgramOperation{
			{
				BoneReferences:  []int{6},
				Timeline:        "rotate",
				Channel:         "value",
				KeyIndices:      []int{1},
				Mode:            "add",
				Operand:         10,
				ExpectedMatches: 2,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "matched 1 keys, expected 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestListAndPatchProjectSlotAttachmentOfficialAlien(t *testing.T) {
	input := filepath.Join("..", "..", "demo", "alien", "alien-human.spine")
	listed, err := ListProjectSlotAttachmentTimelines(input, "death")
	if err != nil {
		t.Fatal(err)
	}
	keyCount := 0
	var selected *spineparser.ProjectSlotAttachmentTimeline
	for index := range listed.Directory.Timelines {
		timeline := &listed.Directory.Timelines[index]
		keyCount += len(timeline.Keys)
		if selected == nil && len(timeline.Keys) >= 2 &&
			timeline.Keys[0].Frame < timeline.Keys[1].Frame {
			selected = timeline
		}
	}
	if len(listed.Directory.Timelines) != 7 || keyCount != 14 || selected == nil {
		t.Fatalf("directory = %#v", listed.Directory)
	}
	from := selected.Keys[0].Frame
	to := from + (selected.Keys[1].Frame-from)/2
	output := filepath.Join(t.TempDir(), "alien-agent.spine")
	patched, err := PatchProjectSlotAttachment(ProjectSlotAttachmentOptions{
		InputPath:  input,
		OutputPath: output,
		Animation:  "death",
		Edits: []spineparser.ProjectSlotAttachmentFrameEdit{
			{
				SlotReference:     selected.SlotReference,
				TimelineReference: selected.TimelineReference,
				TimelineOffset:    selected.Offset,
				KeyIndex:          0,
				From:              from,
				To:                to,
			},
		},
		Apply: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !patched.Applied ||
		patched.Patch.TargetAnimation != "death-agent" ||
		len(patched.Patch.Changes) != 1 {
		t.Fatalf("patched = %#v", patched)
	}
	rediscovered, err := ListProjectSlotAttachmentTimelines(
		output,
		"death-agent",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, timeline := range rediscovered.Directory.Timelines {
		if timeline.SlotReference == selected.SlotReference &&
			timeline.TimelineReference == selected.TimelineReference &&
			timeline.Keys[0].Frame == to {
			return
		}
	}
	t.Fatal("patched slot attachment timeline not found")
}

func mapsEqual(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func rotateValue(
	directory *spineparser.ProjectRotateTimelineDirectory,
	boneReference int,
	keyIndex int,
) (float32, bool) {
	for _, timeline := range directory.Timelines {
		if timeline.BoneReference == boneReference &&
			keyIndex >= 0 && keyIndex < len(timeline.Keys) {
			return timeline.Keys[keyIndex].Value, true
		}
	}
	return 0, false
}
