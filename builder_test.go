package codemap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRepository(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "builder_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a dummy go project
	files := map[string]string{
		"main.go":     "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }",
		"lib/util.go": "package lib\n\nfunc Helper() {}",
		"README.md":   "not a go file",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
	}

	t.Run("default builder", func(t *testing.T) {
		repo, err := BuildRepository(tmpDir)
		if err != nil {
			t.Fatalf("BuildRepository failed: %v", err)
		}

		if len(repo.Files) != 2 {
			t.Errorf("expected 2 files, got %d", len(repo.Files))
		}
	})

	t.Run("concurrent builder with context", func(t *testing.T) {
		builder := NewRepositoryBuilder(WithWorkers(2))
		repo, err := builder.Build(context.Background(), tmpDir)
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		if len(repo.Files) != 2 {
			t.Errorf("expected 2 files, got %d", len(repo.Files))
		}
	})
}

func TestGraphBuilder(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "graph_builder_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a dummy go project with internal imports
	moduleName := "example.com/testrepo"
	files := map[string]string{
		"go.mod":      "module " + moduleName + "\n\ngo 1.25",
		"main.go":     "package main\n\nimport \"" + moduleName + "/lib\"\n\nfunc main() { lib.Helper() }",
		"lib/util.go": "package lib\n\nfunc Helper() {}",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
	}

	repo, err := BuildRepository(tmpDir)
	if err != nil {
		t.Fatalf("BuildRepository failed: %v", err)
	}

	gb := &GraphBuilder{}
	graph := gb.Build(repo)

	if len(graph.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(graph.Nodes))
	}

	// We expect one edge: main.go -> lib/util.go
	if len(graph.Edges) != 1 {
		for i, edge := range graph.Edges {
			t.Logf("Edge %d: %s -> %s", i, edge.From.FilePath, edge.To.FilePath)
		}
		t.Errorf("expected 1 edge, got %d", len(graph.Edges))
	} else {
		edge := graph.Edges[0]
		if !strings.HasSuffix(edge.From.FilePath, "main.go") {
			t.Errorf("expected edge from main.go, got %s", edge.From.FilePath)
		}
		if !strings.HasSuffix(edge.To.FilePath, "util.go") {
			t.Errorf("expected edge to util.go, got %s", edge.To.FilePath)
		}
	}
}

func TestEntrypointDetector(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "entrypoint_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	files := map[string]string{
		"main.go":    "package main\n\nfunc main() {}",
		"cmd/app.go": "package main\n\nfunc main() {}",
		"lib/lib.go": "package lib\n\nfunc Helper() {}",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
	}

	repo, err := BuildRepository(tmpDir)
	if err != nil {
		t.Fatalf("BuildRepository failed: %v", err)
	}

	ed := &EntrypointDetector{}
	entrypoints := ed.Find(repo)

	if len(entrypoints) != 2 {
		t.Errorf("expected 2 entrypoints, got %d", len(entrypoints))
	}

	foundMain := false
	foundApp := false
	for _, ep := range entrypoints {
		if strings.HasSuffix(ep, "main.go") {
			foundMain = true
		}
		if strings.HasSuffix(ep, "app.go") {
			foundApp = true
		}
	}

	if !foundMain || !foundApp {
		t.Errorf("did not find expected entrypoints: main.go=%v, app.go=%v", foundMain, foundApp)
	}
}
