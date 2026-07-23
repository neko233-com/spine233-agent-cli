package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunSummarize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hero.json")
	if err := os.WriteFile(path, []byte(`{"bones":[{"name":"root"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := new(bytes.Buffer)
	if err := Run(context.Background(), []string{"summarize", "--file", path}, bytes.NewReader(nil), output, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["kind"] != "skeleton-json" {
		t.Fatalf("result = %s", output.String())
	}
}

func TestRunAnimateProjectRecipePreview(t *testing.T) {
	recipe := filepath.Join("..", "..", "demo", "hero", "agent-animation.json")
	output := new(bytes.Buffer)
	if err := Run(
		context.Background(),
		[]string{"animate-project-transform", "--recipe", recipe},
		bytes.NewReader(nil),
		output,
		new(bytes.Buffer),
	); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Applied bool `json:"applied"`
		Patch   struct {
			Animation       string `json:"animation"`
			TargetAnimation string `json:"targetAnimation"`
		} `json:"patch"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Applied || result.Patch.Animation != "attack" ||
		result.Patch.TargetAnimation != "attack-agent" {
		t.Fatalf("result = %s", output.String())
	}
}

func TestParseIntegerList(t *testing.T) {
	values, err := parseIntegerList(" 6, 12,40 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 || values[0] != 6 || values[1] != 12 || values[2] != 40 {
		t.Fatalf("values = %#v", values)
	}
	if _, err := parseIntegerList("6,body"); err == nil {
		t.Fatal("invalid integer accepted")
	}
}

func TestRunProgramProjectTransformPreview(t *testing.T) {
	input := filepath.Join("..", "..", "demo", "hero", "hero-human.spine")
	operations := `[{
		"boneReferences":[6],
		"timeline":"rotate",
		"channel":"value",
		"keyIndices":[1],
		"mode":"add",
		"operand":10,
		"expectedMatches":1
	}]`
	output := new(bytes.Buffer)
	if err := Run(
		context.Background(),
		[]string{
			"program-project-transform",
			"--file", input,
			"--animation", "attack",
			"--operations", operations,
		},
		bytes.NewReader(nil),
		output,
		new(bytes.Buffer),
	); err != nil {
		t.Fatal(err)
	}
	var result struct {
		ExpandedEdits []any `json:"expandedEdits"`
		Result        struct {
			Applied bool `json:"applied"`
			Patch   struct {
				TargetAnimation string `json:"targetAnimation"`
			} `json:"patch"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Result.Applied ||
		result.Result.Patch.TargetAnimation != "attack-agent" ||
		len(result.ExpandedEdits) != 1 {
		t.Fatalf("result = %s", output.String())
	}
}
