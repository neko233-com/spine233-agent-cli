package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestServeInitializeAndToolsList(t *testing.T) {
	input := bytes.NewBufferString(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n",
	)
	output := new(bytes.Buffer)
	if err := Serve(context.Background(), input, output); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(output)
	var initialized map[string]any
	var listed map[string]any
	if err := decoder.Decode(&initialized); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&listed); err != nil {
		t.Fatal(err)
	}
	tools := listed["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 9 {
		t.Fatalf("tool count = %d", len(tools))
	}
}

func TestServeToolCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hero.json")
	if err := os.WriteFile(path, []byte(`{"bones":[{"name":"root"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{
		"name":      "spine_summarize",
		"arguments": map[string]any{"path": path},
	})
	request, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": json.RawMessage(params),
	})
	output := new(bytes.Buffer)
	if err := Serve(context.Background(), bytes.NewReader(append(request, '\n')), output); err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	result := response["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("tool returned error: %s", output.String())
	}
}
