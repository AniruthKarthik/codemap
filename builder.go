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

	// Hydrate symbols and classify files
	classifier := &Classifier{}
	allPaths := make([]string, 0, len(files))
	for _, f := range files {
		allPaths = append(allPaths, f.Path)
	}
	commonRoot := findCommonRoot(allPaths)
	moduleName := getModuleName(commonRoot)

	for _, f := range files {
		f.Role = classifier.Classify(f)

		relPath, _ := filepath.Rel(commonRoot, f.Path)
		dir := filepath.Dir(relPath)
		var pkgPath string
		if dir == "." {
			pkgPath = moduleName
		} else {
			pkgPath = filepath.Join(moduleName, dir)
		}
		pkgPath = filepath.ToSlash(pkgPath)

		for i := range f.Symbols {
			sym := &f.Symbols[i]
			sym.Package = pkgPath
			if sym.Kind == models.MethodSymbol && sym.Receiver != "" {
				sym.ID = fmt.Sprintf("%s.%s.%s", pkgPath, sym.Receiver, sym.Name)
			} else {
				sym.ID = fmt.Sprintf("%s.%s", pkgPath, sym.Name)
			}
		}
	}

	// 2. Build Symbol Reference Graph
	var symbolEdges []models.SymbolEdge
	// Map of (packagePath, name) -> ID
	symbolMap := make(map[string]string)
	for _, f := range files {
		for _, sym := range f.Symbols {
			// For structs/interfaces/functions, key is pkgPath.Name
			// For methods, we don't usually reference them by name directly in type refs,
			// but we can add them just in case.
			key := fmt.Sprintf("%s.%s", sym.Package, sym.Name)
			symbolMap[key] = sym.ID
		}
	}

	for _, f := range files {
		// Map of import alias (or package name) to full path
		importMap := make(map[string]string)
		for _, imp := range f.Imports {
			parts := strings.Split(imp, "/")
			pkgName := parts[len(parts)-1]
			importMap[pkgName] = imp
		}

		relPath, _ := filepath.Rel(commonRoot, f.Path)
		dir := filepath.Dir(relPath)
		var pkgPath string
		if dir == "." {
			pkgPath = moduleName
		} else {
			pkgPath = filepath.Join(moduleName, dir)
		}
		pkgPath = filepath.ToSlash(pkgPath)

		for _, sym := range f.Symbols {
			for _, ref := range sym.References {
				var targetID string
				if strings.Contains(ref, ".") {
					// Qualified reference: pkg.Type
					parts := strings.Split(ref, ".")
					alias := parts[0]
					typeName := parts[1]
					if fullPath, ok := importMap[alias]; ok {
						key := fmt.Sprintf("%s.%s", fullPath, typeName)
						targetID = symbolMap[key]
					}
				} else {
					// Local reference: Type
					key := fmt.Sprintf("%s.%s", pkgPath, ref)
					targetID = symbolMap[key]
				}

				if targetID != "" {
					symbolEdges = append(symbolEdges, models.SymbolEdge{
						From: sym.ID,
						To:   targetID,
					})
				}
			}
		}
	}

	return &models.Repository{
		Files:       files,
		SymbolEdges: symbolEdges,
	}, nil
}

// Classifier determines the role of a file in the project structure.
type Classifier struct{}

// Classify assigns a FileRole to a file based on its path and contents.
func (c *Classifier) Classify(f *models.File) models.FileRole {
	path := strings.ToLower(f.Path)

	if strings.HasSuffix(path, "_test.go") {
		return models.RoleTest
	}

	// Check for common directories
	dirParts := strings.Split(path, string(filepath.Separator))
	for _, part := range dirParts {
		switch part {
		case "examples", "example":
			return models.RoleExample
		case "testdata", "fixtures", "mock", "mocks":
			return models.RoleTest
		case "vendor":
			return models.RoleInfrastructure
		case "generated", "gen":
			return models.RoleGenerated
		}
	}

	// Entrypoint detection
	for _, fn := range f.Functions {
		if fn.Name == "main" && f.Package == "package main" {
			return models.RoleEntrypoint
		}
	}

	// Default to core logic for now
	return models.RoleCoreLogic
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

	// 1. Initialize Symbol Scores
	idToSymbol := make(map[string]*models.Symbol)
	for _, f := range repo.Files {
		for i := range f.Symbols {
			sym := &f.Symbols[i]
			idToSymbol[sym.ID] = sym

			// Base score by kind
			switch sym.Kind {
			case models.StructSymbol:
				sym.Score = 50
			case models.InterfaceSymbol:
				sym.Score = 60
			case models.FunctionSymbol, models.MethodSymbol:
				sym.Score = 20
			}

			// Bonus for exported symbols
			if len(sym.Name) > 0 && sym.Name[0] >= 'A' && sym.Name[0] <= 'Z' {
				sym.Score += 20
			}
		}
	}

	// 2. Apply Centrality (Incoming References)
	for _, edge := range repo.SymbolEdges {
		if target, ok := idToSymbol[edge.To]; ok {
			target.Score += 30
		}
	}

	// 3. Resolve import paths for each file and count package imports (for file-level context)
	paths := make([]string, 0, len(repo.Files))
	for _, f := range repo.Files {
		paths = append(paths, f.Path)
	}
	root := findCommonRoot(paths)
	moduleName := getModuleName(root)

	packageImportCount := make(map[string]int)
	for _, f := range repo.Files {
		for _, imp := range f.Imports {
			packageImportCount[imp]++
		}
	}

	// 4. Score each file based on its symbols and role
	for _, f := range repo.Files {
		var score float64

		// File score is primarily the sum of its architectural symbols
		for _, sym := range f.Symbols {
			score += sym.Score
		}

		// Entrypoint bonus (handled by role but let's keep it explicit for now)
		if f.Role == models.RoleEntrypoint {
			score += 100
		}

		// Import context
		relPath, _ := filepath.Rel(root, f.Path)
		dir := filepath.Dir(relPath)
		var pkgPath string
		if dir == "." {
			pkgPath = moduleName
		} else {
			pkgPath = filepath.Join(moduleName, dir)
		}
		pkgPath = filepath.ToSlash(pkgPath)
		score += float64(packageImportCount[pkgPath]) * 15

		// Apply role-based penalties
		switch f.Role {
		case models.RoleTest:
			score = score * 0.1 // -90% penalty
		case models.RoleExample, models.RoleGenerated:
			score = score * 0.3 // -70% penalty
		case models.RoleInfrastructure:
			score = score * 0.5 // -50% penalty
		}

		f.Score = int64(score)

		// 5. Update File.Blocks from symbols (replacing old function-only blocks)
		f.Blocks = make([]models.CodeBlock, 0, len(f.Symbols))
		for _, sym := range f.Symbols {
			f.Blocks = append(f.Blocks, models.CodeBlock{
				Name:      sym.Name,
				StartLine: sym.StartLine,
				EndLine:   sym.EndLine,
				Score:     sym.Score,
			})
		}
	}

	// 6. Sort files by score descending
	sort.Slice(repo.Files, func(i, j int) bool {
		return repo.Files[i].Score > repo.Files[j].Score
	})
}

// Generator creates a sequential learning path for exploring a repository.
type Generator struct{}

// Generate produces a list of LearningSteps based on file rankings and entrypoints.
func (g *Generator) Generate(repo *models.Repository) []models.LearningStep {
	steps := make([]models.LearningStep, 0, len(repo.Files))

	for i, f := range repo.Files {
		reason := "Key repository component"

		// Determine reason based on file characteristics
		isEntrypoint := false
		for _, fn := range f.Functions {
			if fn.Name == "main" && f.Package == "package main" {
				isEntrypoint = true
				break
			}
		}

		if isEntrypoint {
			reason = "Application entrypoint"
		} else if f.Score > 50 {
			reason = "Core business logic or utility"
		} else {
			reason = "Supporting implementation detail"
		}

		steps = append(steps, models.LearningStep{
			Order:  i + 1,
			File:   f.Path,
			Reason: reason,
			Score:  float64(f.Score),
		})
	}

	return steps
}
