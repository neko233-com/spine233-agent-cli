package spineops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
