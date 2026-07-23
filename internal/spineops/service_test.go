package spineops

import (
	"encoding/json"
	"os"
	"path/filepath"
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
