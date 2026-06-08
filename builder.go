package codemap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/AniruthKarthik/codemap/internal/models"
	"github.com/AniruthKarthik/codemap/parser"
	"github.com/AniruthKarthik/codemap/scanner"
	"golang.org/x/sync/errgroup"
)

// RepositoryBuilder orchestrates the scanning and parsing of a repository.
type RepositoryBuilder struct {
	scanner scanner.Scanner
	parser  parser.Parser
	workers int
}

// BuilderOption defines a functional option for configuring a RepositoryBuilder.
type BuilderOption func(*RepositoryBuilder)

// WithScanner sets a custom scanner for the builder.
func WithScanner(s scanner.Scanner) BuilderOption {
	return func(b *RepositoryBuilder) {
		b.scanner = s
	}
}

// WithParser sets a custom parser for the builder.
func WithParser(p parser.Parser) BuilderOption {
	return func(b *RepositoryBuilder) {
		b.parser = p
	}
}

// WithWorkers sets the number of concurrent workers for parsing.
func WithWorkers(n int) BuilderOption {
	return func(b *RepositoryBuilder) {
		if n > 0 {
			b.workers = n
		}
	}
}

// NewRepositoryBuilder creates a new RepositoryBuilder with default configurations.
func NewRepositoryBuilder(opts ...BuilderOption) *RepositoryBuilder {
	b := &RepositoryBuilder{
		scanner: scanner.NewScanner(),
		parser:  parser.NewGoParser(),
		workers: runtime.NumCPU(),
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// Build walks the filesystem starting from root and builds a Repository model concurrently.
func (b *RepositoryBuilder) Build(ctx context.Context, root string) (*models.Repository, error) {
	paths, err := b.scanner.Scan(root)
	if err != nil {
		return nil, fmt.Errorf("failed to scan repository: %w", err)
	}

	pathChan := make(chan string, len(paths))
	for _, path := range paths {
		pathChan <- path
	}
	close(pathChan)

	// We use a mutex to safely append to the results slice from multiple goroutines.
	// Alternatively, we could use a channel, but a mutex is efficient for simple appends.
	var mu sync.Mutex
	files := make([]*models.File, 0, len(paths))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(b.workers)

	for path := range pathChan {
		path := path // capture range variable
		g.Go(func() error {
			file, err := b.parser.Parse(path)
			if err != nil {
				return fmt.Errorf("failed to parse %q: %w", path, err)
			}

			mu.Lock()
			files = append(files, file)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &models.Repository{
		Files: files,
	}, nil
}

// BuildRepository is a convenience function that uses the default RepositoryBuilder.
func BuildRepository(root string) (*models.Repository, error) {
	builder := NewRepositoryBuilder()
	return builder.Build(context.Background(), root)
}

// GraphBuilder constructs a dependency graph from a Repository.
type GraphBuilder struct{}

// Build creates a dependency graph where edges represent package imports.
func (gb *GraphBuilder) Build(repo *models.Repository) *models.Graph {
	graph := &models.Graph{
		Nodes: make([]models.Node, 0, len(repo.Files)),
		Edges: make([]models.Edge, 0),
	}

	if len(repo.Files) == 0 {
		return graph
	}

	// Map to quickly find files by their import path.
	importPathToFiles := make(map[string][]*models.File)

	// First, find the common root of all files to determine relative paths.
	paths := make([]string, 0, len(repo.Files))
	for _, f := range repo.Files {
		paths = append(paths, f.Path)
	}
	root := findCommonRoot(paths)
	moduleName := getModuleName(root)

	for _, f := range repo.Files {
		graph.Nodes = append(graph.Nodes, models.Node{FilePath: f.Path})

		relPath, _ := filepath.Rel(root, f.Path)
		dir := filepath.Dir(relPath)

		var pkgPath string
		if dir == "." {
			pkgPath = moduleName
		} else {
			pkgPath = filepath.Join(moduleName, dir)
		}
		// Normalize for Go (forward slashes)
		pkgPath = filepath.ToSlash(pkgPath)
		importPathToFiles[pkgPath] = append(importPathToFiles[pkgPath], f)
	}

	// For each file, check its imports and create edges to files in those packages.
	for _, f := range repo.Files {
		fromNode := models.Node{FilePath: f.Path}
		for _, imp := range f.Imports {
			if targetFiles, ok := importPathToFiles[imp]; ok {
				for _, targetFile := range targetFiles {
					// Avoid self-edges if they occur (rare in Go packages)
					if f.Path == targetFile.Path {
						continue
					}
					graph.Edges = append(graph.Edges, models.Edge{
						From: fromNode,
						To:   models.Node{FilePath: targetFile.Path},
					})
				}
			}
		}
	}

	return graph
}

// findCommonRoot identifies the deepest shared directory among a set of file paths.
func findCommonRoot(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	root := filepath.Dir(paths[0])
	for _, p := range paths[1:] {
		for !strings.HasPrefix(p, root+string(filepath.Separator)) && p != root {
			newRoot := filepath.Dir(root)
			if newRoot == root {
				break
			}
			root = newRoot
		}
	}
	return root
}

// getModuleName parses the go.mod file in the root directory to find the module name.
func getModuleName(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

// EntrypointDetector identifies entrypoints (main functions) in a Repository.
type EntrypointDetector struct{}

// Find scans all files in the repository and returns paths to those containing a func main().
func (ed *EntrypointDetector) Find(repo *models.Repository) []string {
	var entrypoints []string
	for _, f := range repo.Files {
		for _, fn := range f.Functions {
			if fn.Name == "main" && f.Package == "package main" {
				entrypoints = append(entrypoints, f.Path)
				break
			}
		}
	}
	return entrypoints
}

// Ranker calculates importance scores for files and sorts them.
type Ranker struct{}

// Rank assigns a score to each file in the repository and sorts repo.Files by score descending.
func (r *Ranker) Rank(repo *models.Repository) {
	if len(repo.Files) == 0 {
		return
	}

	// 1. Resolve import paths for each file and count package imports
	paths := make([]string, 0, len(repo.Files))
	for _, f := range repo.Files {
		paths = append(paths, f.Path)
	}
	root := findCommonRoot(paths)
	moduleName := getModuleName(root)

	// Map of file path to its resolved package import path
	fileToPkgPath := make(map[string]string)
	// Map of package import path to count of files importing it
	packageImportCount := make(map[string]int)

	for _, f := range repo.Files {
		relPath, _ := filepath.Rel(root, f.Path)
		dir := filepath.Dir(relPath)
		var pkgPath string
		if dir == "." {
			pkgPath = moduleName
		} else {
			pkgPath = filepath.Join(moduleName, dir)
		}
		pkgPath = filepath.ToSlash(pkgPath)
		fileToPkgPath[f.Path] = pkgPath

		for _, imp := range f.Imports {
			packageImportCount[imp]++
		}
	}

	// 2. Score each file
	for _, f := range repo.Files {
		var score int64

		// +100 contains main()
		for _, fn := range f.Functions {
			if fn.Name == "main" && f.Package == "package main" {
				score += 100
				break
			}
		}

		// +20 exported functions
		for _, fn := range f.Functions {
			if fn.Exported {
				score += 20
			}
		}

		// +10 import count
		score += int64(len(f.Imports)) * 10

		// +15 imported by others
		pkgPath := fileToPkgPath[f.Path]
		score += int64(packageImportCount[pkgPath]) * 15

		f.Score = score
	}

	// 3. Sort files by score descending
	sort.Slice(repo.Files, func(i, j int) bool {
		return repo.Files[i].Score > repo.Files[j].Score
	})
}
