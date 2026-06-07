package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanner_Scan(t *testing.T) {
	// Create a temporary directory structure
	tmpDir, err := os.MkdirTemp("", "scanner_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files and directories
	files := []string{
		"main.go",
		"internal/utils.go",
		"vendor/dependency.go",
		".git/config",
		"node_modules/library.js",
		"README.md",
		"custom/ignored.go",
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f)
		err := os.MkdirAll(filepath.Dir(path), 0755)
		if err != nil {
			t.Fatalf("Failed to create dir for %s: %v", f, err)
		}
		err = os.WriteFile(path, []byte("test"), 0644)
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", f, err)
		}
	}

	t.Run("default options", func(t *testing.T) {
		s := NewScanner()
		got, err := s.Scan(tmpDir)
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}

		want := []string{
			filepath.Join(tmpDir, "main.go"),
			filepath.Join(tmpDir, "internal/utils.go"),
			filepath.Join(tmpDir, "custom/ignored.go"),
		}

		compareResults(t, got, want)
	})

	t.Run("custom excluded dirs", func(t *testing.T) {
		s := NewScanner(WithExcludedDirs([]string{"custom", ".git"}))
		got, err := s.Scan(tmpDir)
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}

		// vendor and node_modules should NOT be excluded now
		want := []string{
			filepath.Join(tmpDir, "main.go"),
			filepath.Join(tmpDir, "internal/utils.go"),
			filepath.Join(tmpDir, "vendor/dependency.go"),
		}

		compareResults(t, got, want)
	})
}

func compareResults(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("got %d files, want %d: %v", len(got), len(want), got)
		return
	}

	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected file %s not found in results", w)
		}
	}
}
