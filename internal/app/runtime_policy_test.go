package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProductionCodeHasNoProcessOrNetworkRuntime(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, sourceRoot := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(
			filepath.Join(repositoryRoot, sourceRoot),
			func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() ||
					!strings.HasSuffix(entry.Name(), ".go") ||
					strings.HasSuffix(entry.Name(), "_test.go") {
					return nil
				}
				checkRuntimePolicyFile(t, path)
				return nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func checkRuntimePolicyFile(t *testing.T, path string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	blockedImports := map[string]string{
		"net":      "network access",
		"net/http": "HTTP access",
		"os/exec":  "external process execution",
		"syscall":  "raw process or network syscalls",
	}
	for _, imported := range file.Imports {
		name, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatalf("%s: invalid import %s", path, imported.Path.Value)
		}
		if reason, blocked := blockedImports[name]; blocked {
			t.Errorf("%s imports %q (%s)", path, name, reason)
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && identifier.Name == "os" && selector.Sel.Name == "StartProcess" {
			t.Errorf("%s calls os.StartProcess", path)
		}
		return true
	})
}
