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

	s := NewScanner()
	got, err := s.Scan(tmpDir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Expected .go files (relative to tmpDir)
	want := []string{
		filepath.Join(tmpDir, "main.go"),
		filepath.Join(tmpDir, "internal/utils.go"),
	}

	if len(got) != len(want) {
		t.Errorf("got %d files, want %d", len(got), len(want))
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
