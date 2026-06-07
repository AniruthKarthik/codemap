package codemap

import (
	"context"
	"os"
	"path/filepath"
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
