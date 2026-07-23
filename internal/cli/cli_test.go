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
