package scanner

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Scanner defines the behavior for discovering Go source files.
type Scanner interface {
	Scan(root string) ([]string, error)
}

// DefaultScanner is the standard implementation of the Scanner interface.
type DefaultScanner struct {
	excludedDirs map[string]struct{}
}

// Option defines a functional option for configuring a DefaultScanner.
type Option func(*DefaultScanner)

// WithExcludedDirs allows overriding the default excluded directories.
func WithExcludedDirs(dirs []string) Option {
	return func(s *DefaultScanner) {
		s.excludedDirs = make(map[string]struct{}, len(dirs))
		for _, d := range dirs {
			s.excludedDirs[d] = struct{}{}
		}
	}
}

// NewScanner creates a new DefaultScanner with standard defaults.
func NewScanner(opts ...Option) *DefaultScanner {
	s := &DefaultScanner{
		excludedDirs: map[string]struct{}{
			".git":         {},
			"vendor":       {},
			"node_modules": {},
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Scan walks the filesystem starting from root, skipping excluded directories,
// and returns a slice of paths to .go files.
func (s *DefaultScanner) Scan(root string) ([]string, error) {
	var goFiles []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %q: %w", path, err)
		}

		if d.IsDir() {
			if _, excluded := s.excludedDirs[d.Name()]; excluded {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(path, ".go") {
			goFiles = append(goFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("filesystem scan failed: %w", err)
	}

	return goFiles, nil
}
