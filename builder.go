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
				if strings.Contains(ref.Name, ".") {
					// Qualified reference: pkg.Type
					parts := strings.Split(ref.Name, ".")
					alias := parts[0]
					typeName := parts[1]
					if fullPath, ok := importMap[alias]; ok {
						key := fmt.Sprintf("%s.%s", fullPath, typeName)
						targetID = symbolMap[key]
					}
				} else {
					// Local reference: Type
					key := fmt.Sprintf("%s.%s", pkgPath, ref.Name)
					targetID = symbolMap[key]
				}

				if targetID != "" {
					symbolEdges = append(symbolEdges, models.SymbolEdge{
						From: sym.ID,
						To:   targetID,
						Type: ref.Type,
					})
				}
			}
		}
	}

	// 3. Interface Implementation Detection (Heuristic)
	var interfaces []*models.Symbol
	var structs []*models.Symbol
	structMethods := make(map[string]map[string]bool) // structID -> methodNames

	for _, f := range files {
		for i := range f.Symbols {
			sym := &f.Symbols[i]
			if sym.Kind == models.InterfaceSymbol {
				interfaces = append(interfaces, sym)
			} else if sym.Kind == models.StructSymbol {
				structs = append(structs, sym)
			} else if sym.Kind == models.MethodSymbol && sym.Receiver != "" {
				pkgPath := sym.Package
				structKey := fmt.Sprintf("%s.%s", pkgPath, sym.Receiver)
				structID := symbolMap[structKey]
				if structID != "" {
					if structMethods[structID] == nil {
						structMethods[structID] = make(map[string]bool)
					}
					structMethods[structID][sym.Name] = true
				}
			}
		}
	}

	for _, iface := range interfaces {
		if len(iface.References) == 0 {
			continue
		}
		for _, str := range structs {
			matches := true
			hasMethods := false
			for _, ref := range iface.References {
				if ref.Type == models.RefCall {
					hasMethods = true
					if !structMethods[str.ID][ref.Name] {
						matches = false
						break
					}
				}
			}
			if matches && hasMethods {
				symbolEdges = append(symbolEdges, models.SymbolEdge{
					From: str.ID,
					To:   iface.ID,
					Type: models.RefImplement,
				})
			}
		}
	}

	repo := &models.Repository{
		Files:       files,
		SymbolEdges: symbolEdges,
	}

	ranker := &Ranker{}
	ranker.Rank(repo)

	detector := &ConceptDetector{}
	repo.Concepts, repo.ConceptEdges = detector.Detect(repo)

	// Populate Rankings
	// 1. Architecture Ranking (Concepts are already sorted by Importance in Detect)
	repo.Rankings.ArchitectureRanking = make([]string, len(repo.Concepts))
	for i, c := range repo.Concepts {
		repo.Rankings.ArchitectureRanking[i] = c.ID
	}

	// 2. Learning Ranking (Use Generator to calculate priorities and handle prerequisites)
	generator := &Generator{}
	generator.GenerateUnits(repo) // This populates repo.Rankings.LearningRanking

	return repo, nil
}

// ConceptDetector clusters symbols into logical high-level features.
type ConceptDetector struct{}

// Detect analyzes the repository and groups symbols into Concepts.
func (cd *ConceptDetector) Detect(repo *models.Repository) ([]models.Concept, []models.ConceptEdge) {
	if len(repo.Files) == 0 {
		return nil, nil
	}

	// 1. Gather all symbols and their neighborhood
	idToSymbol := make(map[string]*models.Symbol)
	neighbors := make(map[string]map[string]bool)

	for _, f := range repo.Files {
		for i := range f.Symbols {
			sym := &f.Symbols[i]
			idToSymbol[sym.ID] = sym
			if neighbors[sym.ID] == nil {
				neighbors[sym.ID] = make(map[string]bool)
			}
		}
	}

	for _, edge := range repo.SymbolEdges {
		if neighbors[edge.From] != nil {
			neighbors[edge.From][edge.To] = true
		}
		if neighbors[edge.To] != nil {
			neighbors[edge.To][edge.From] = true
		}
	}

	// 2. Initial Clustering by Receiver (Strong Signal)
	conceptsMap := make(map[string]*models.Concept)
	symbolToConceptID := make(map[string]string)

	for _, f := range repo.Files {
		for _, sym := range f.Symbols {
			if sym.Kind == models.MethodSymbol && sym.Receiver != "" {
				conceptName := sym.Receiver
				conceptID := sym.Package + "." + sym.Receiver
				
				count := 0
				if c, ok := conceptsMap[conceptID]; ok {
					count = len(c.SymbolIDs)
				}

				if count >= 100 {
					subIndex := count / 100
					conceptName = fmt.Sprintf("%s (Part %d)", sym.Receiver, subIndex+1)
					conceptID = fmt.Sprintf("%s.Part%d", conceptID, subIndex+1)
				}

				if _, ok := conceptsMap[conceptID]; !ok {
					conceptsMap[conceptID] = &models.Concept{ID: conceptID, Name: conceptName}
				}

				conceptsMap[conceptID].SymbolIDs = append(conceptsMap[conceptID].SymbolIDs, sym.ID)
				
				importance := sym.Score
				switch f.Role {
				case models.RoleTest:
					importance *= 0.1
				case models.RoleExample, models.RoleGenerated:
					importance *= 0.3
				case models.RoleInfrastructure:
					importance *= 0.5
				}
				conceptsMap[conceptID].Importance += importance
				symbolToConceptID[sym.ID] = conceptID
			}
		}
	}

	// 3. Group remaining symbols by neighborhood similarity
	for _, f := range repo.Files {
		for _, sym := range f.Symbols {
			if _, ok := symbolToConceptID[sym.ID]; ok {
				continue
			}

			bestConceptID := ""
			bestScore := 0.0

			for id, concept := range conceptsMap {
				matchCount := 0
				for _, conceptSymID := range concept.SymbolIDs {
					if neighbors[sym.ID][conceptSymID] {
						matchCount++
					}
				}
				
				score := float64(matchCount) / float64(len(concept.SymbolIDs)+1)
				if score > bestScore {
					bestScore = score
					bestConceptID = id
				}
			}

			importance := sym.Score
			switch f.Role {
			case models.RoleTest:
				importance *= 0.1
			case models.RoleExample, models.RoleGenerated:
				importance *= 0.3
			case models.RoleInfrastructure:
				importance *= 0.5
			}

			if bestScore > 0.1 && len(conceptsMap[bestConceptID].SymbolIDs) < 30 {
				conceptsMap[bestConceptID].SymbolIDs = append(conceptsMap[bestConceptID].SymbolIDs, sym.ID)
				conceptsMap[bestConceptID].Importance += importance
				symbolToConceptID[sym.ID] = bestConceptID
			} else {
				conceptName := filepath.Base(f.Path)
				conceptID := f.Path
				if _, ok := conceptsMap[conceptID]; !ok {
					conceptsMap[conceptID] = &models.Concept{ID: conceptID, Name: conceptName}
				}
				conceptsMap[conceptID].SymbolIDs = append(conceptsMap[conceptID].SymbolIDs, sym.ID)
				conceptsMap[conceptID].Importance += importance
				symbolToConceptID[sym.ID] = conceptID
			}
		}
	}

	// 4. Build Concept Dependencies and Calculate Cohesion
	conceptEdgesMap := make(map[string]bool)
	var conceptEdges []models.ConceptEdge

	for _, edge := range repo.SymbolEdges {
		fromConceptID := symbolToConceptID[edge.From]
		toConceptID := symbolToConceptID[edge.To]

		if fromConceptID != "" && toConceptID != "" && fromConceptID != toConceptID {
			edgeKey := fromConceptID + "->" + toConceptID
			if !conceptEdgesMap[edgeKey] {
				conceptEdges = append(conceptEdges, models.ConceptEdge{
					From: fromConceptID,
					To:   toConceptID,
				})
				conceptEdgesMap[edgeKey] = true
				
				// dependency: from uses to, so to is prerequisite for from
				conceptsMap[fromConceptID].PrerequisiteIDs = append(conceptsMap[fromConceptID].PrerequisiteIDs, toConceptID)
			}
		}
	}

	var concepts []models.Concept
	for _, c := range conceptsMap {
		if len(c.SymbolIDs) == 0 {
			continue
		}

		preSet := make(map[string]bool)
		var cleanPres []string
		for _, preID := range c.PrerequisiteIDs {
			if !preSet[preID] {
				cleanPres = append(cleanPres, preID)
				preSet[preID] = true
			}
		}
		c.PrerequisiteIDs = cleanPres

		// Cohesion calculation
		internalEdges := 0
		symSet := make(map[string]bool)
		for _, id := range c.SymbolIDs {
			symSet[id] = true
			
			// Aggregate symbol factors
			if sym, ok := idToSymbol[id]; ok {
				c.Factors.Centrality += sym.Factors.Centrality
				c.Factors.Reachability += sym.Factors.Reachability
				
				// Bonus for exported, non-test symbols
				if len(sym.Name) > 0 && sym.Name[0] >= 'A' && sym.Name[0] <= 'Z' && !strings.HasPrefix(sym.Name, "Test") {
					c.Factors.PublicAPIWeight += 100
				}
			}
		}

		// Normalize API weight: core APIs usually have a focused set of important methods
		if c.Factors.PublicAPIWeight > 2000 {
			c.Factors.PublicAPIWeight = 2000 + (c.Factors.PublicAPIWeight-2000)*0.1
		}

		for _, fromID := range c.SymbolIDs {
			for toID := range neighbors[fromID] {
				if symSet[toID] {
					internalEdges++
				}
			}
		}
		n := len(c.SymbolIDs)
		if n > 1 {
			c.Cohesion = float64(internalEdges) / float64(n*(n-1))
		} else {
			c.Cohesion = 1.0
		}
		c.Factors.ConceptSize = float64(n)

		// Role Classification
		c.Role = models.RoleSupporting
		if c.Importance > 1000 {
			c.Role = models.RoleFoundational
		} else if c.Importance > 300 {
			c.Role = models.RoleCore
		} else if strings.Contains(strings.ToLower(c.Name), "test") || strings.Contains(strings.ToLower(c.Name), "mock") {
			c.Role = models.RolePeripheral
		}

		// Concept Naming (Use the most important symbol's name)
		bestSymName := ""
		bestSymScore := -1.0
		for _, id := range c.SymbolIDs {
			if sym, ok := idToSymbol[id]; ok {
				if sym.Score > bestSymScore {
					bestSymScore = sym.Score
					bestSymName = sym.Name
				}
			}
		}
		if bestSymName != "" && (c.Name == "" || strings.HasSuffix(c.Name, ".go") || strings.HasSuffix(c.Name, " Logic")) {
			if c.Role == models.RoleFoundational {
				c.Name = bestSymName
			} else {
				c.Name = bestSymName + " Module"
			}
		}
		// Summary Generation (Heuristic - Disabled by default for UI, will be generated by AI on demand)
		c.Summary = models.ConceptSummary{
		        Purpose:           "",
		        WhyLearn:          "",
		        LearningObjective: "",
		}

		concepts = append(concepts, *c)
	}

	// Final pass to populate Unlocks/UsedBy for summaries and ImportanceFactors
	unlocksMap := make(map[string][]string) // ToID -> FromIDs (who depends on me)
	usedByMap := make(map[string][]string)  // FromID -> ToIDs (who I depend on)
	for _, edge := range conceptEdges {
		unlocksMap[edge.To] = append(unlocksMap[edge.To], conceptsMap[edge.From].Name)
		usedByMap[edge.From] = append(usedByMap[edge.From], conceptsMap[edge.To].Name)
	}

	for i := range concepts {
		c := &concepts[i]
		c.Summary.Unlocks = unlocksMap[c.ID]
		c.Factors.DependencyUnlocks = float64(len(unlocksMap[c.ID]))
		
		hasMethods := false
		for _, symID := range c.SymbolIDs {
			if sym, ok := idToSymbol[symID]; ok {
				if sym.Kind == models.MethodSymbol || sym.Kind == models.FunctionSymbol {
					hasMethods = true
					break
				}
			}
		}

		// Mental Model Detection (Heuristics)
		c.Type = models.SupportingType
		
		// Core Abstraction: Exported Struct with many methods and high centrality
		// Main API: Entry points and high-unlock concepts
		if hasMethods && c.Factors.PublicAPIWeight > 500 && c.Factors.Centrality > 200 {
			c.Type = models.CoreAbstraction
		} else if hasMethods && c.Factors.ConceptSize > 5 && c.Factors.PublicAPIWeight > 300 {
			c.Type = models.CoreAbstraction
		} else if hasMethods && c.Factors.DependencyUnlocks > 10 {
			c.Type = models.EntryAPI
		} else if c.Importance < 100 || !hasMethods {
			c.Type = models.UtilityType
		}

		// Semantic Explanation refinement
		if len(usedByMap[c.ID]) > 0 {
			c.Summary.WhyLearn += fmt.Sprintf(" Used by %s.", strings.Join(usedByMap[c.ID], ", "))
		}
	}

	sort.Slice(concepts, func(i, j int) bool {
		return concepts[i].Importance > concepts[j].Importance
	})

	return concepts, conceptEdges
}

// Classifier determines the role of a file in the project structure.
type Classifier struct{}

// Classify assigns a FileRole to a file based on its path and contents.
func (c *Classifier) Classify(f *models.File) models.FileRole {
	path := strings.ToLower(f.Path)

	if strings.HasSuffix(path, "_test.go") {
		return models.RoleTest
	}

	// Check for common directories and file names
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
		case "internal", "pkg", "core", "domain":
			// These are hints but we need more specific classification
		}
	}

	// Specific file name patterns
	base := filepath.Base(path)
	if strings.Contains(base, "config") || strings.Contains(base, "settings") {
		return models.RoleConfig
	}
	if strings.Contains(base, "util") || strings.Contains(base, "helper") || base == "errors.go" || base == "constants.go" {
		return models.RoleUtility
	}
	if strings.Contains(base, "model") || strings.Contains(base, "types") || base == "schema.go" {
		return models.RoleCoreDomain
	}
	if strings.Contains(base, "executor") || strings.Contains(base, "workflow") || strings.Contains(base, "engine") || strings.Contains(base, "runner") {
		return models.RoleExecution
	}
	if strings.Contains(base, "agent") {
		return models.RoleAgent
	}
	if strings.Contains(base, "provider") || strings.Contains(base, "adapter") {
		return models.RoleProvider
	}
	if strings.Contains(base, "store") || strings.Contains(base, "db") || strings.Contains(base, "repository") || strings.Contains(base, "persistence") {
		return models.RolePersistence
	}

	// Entrypoint detection
	for _, fn := range f.Functions {
		if fn.Name == "main" && f.Package == "package main" {
			return models.RoleEntrypoint
		}
	}
	if strings.Contains(path, "cmd/") && strings.HasSuffix(path, "main.go") {
		return models.RoleEntrypoint
	}

	// Default to core domain for now if it contains many symbols
	if len(f.Symbols) > 5 {
		return models.RoleCoreDomain
	}

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
		if f.Role == models.RoleEntrypoint {
			entrypoints = append(entrypoints, f.Path)
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
				sym.Score = 300 // Data structures are high
			case models.InterfaceSymbol:
				sym.Score = 400 // Interfaces are higher
			case models.FunctionSymbol, models.MethodSymbol:
				sym.Score = 200 // Behavior is important but usually tied to a struct
			}

			// Bonus for exported symbols
			if len(sym.Name) > 0 && sym.Name[0] >= 'A' && sym.Name[0] <= 'Z' {
				sym.Score += 100
			}
		}
	}

	// 2. Apply Centrality (Incoming References with typed weights)
	for _, edge := range repo.SymbolEdges {
		if target, ok := idToSymbol[edge.To]; ok {
			var weight float64
			switch edge.Type {
			case models.RefCall:
				weight = 50 // Behavior call is very important
			case models.RefImplement:
				weight = 40
			case models.RefConstruct:
				weight = 40
			case models.RefField:
				weight = 20
			default:
				weight = 10
			}
			target.Factors.Centrality += weight
			target.Score += weight
		}
	}

	// 3. Execution Flow Bonus (BFS from entrypoints)
	distances := make(map[string]int)
	queue := []string{}

	// Adjacency list for BFS (From -> To)
	adj := make(map[string][]string)
	for _, edge := range repo.SymbolEdges {
		adj[edge.From] = append(adj[edge.From], edge.To)
	}

	for _, f := range repo.Files {
		if f.Role == models.RoleEntrypoint {
			for _, sym := range f.Symbols {
				// Any symbol in an entrypoint file can be a start for reachability
				distances[sym.ID] = 0
				queue = append(queue, sym.ID)
			}
		}
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		dist := distances[curr]

		for _, neighbor := range adj[curr] {
			if _, seen := distances[neighbor]; !seen {
				distances[neighbor] = dist + 1
				queue = append(queue, neighbor)
				
				if target, ok := idToSymbol[neighbor]; ok {
					// Massive reachability bonus to favor execution flow
					bonus := 2000.0 / float64(dist+1)
					target.Factors.Reachability += bonus
					target.Score += bonus
				}
			}
		}
	}

	// 4. File Level Scoring
	for _, f := range repo.Files {
		var score float64
		var mentalModelScore float64

		// Sum top symbols' scores
		sort.Slice(f.Symbols, func(i, j int) bool {
			return f.Symbols[i].Score > f.Symbols[j].Score
		})
		
		for i, sym := range f.Symbols {
			// Diminishing returns for many small symbols in a file
			if i < 5 {
				score += sym.Score
			} else {
				score += sym.Score * 0.1
			}
		}

		// Mental Model Scoring (Behavior vs Data)
		behaviorCount := 0
		dataCount := 0
		for _, sym := range f.Symbols {
			if sym.Kind == models.FunctionSymbol || sym.Kind == models.MethodSymbol {
				behaviorCount++
			} else {
				dataCount++
			}
		}
		
		if behaviorCount > 0 && behaviorCount >= dataCount {
			mentalModelScore += 1000 // Explains behavior
		}
		
		// Role-based Layer Scoring
		switch f.Role {
		case models.RoleEntrypoint:
			score += 20000 
		case models.RoleCoreDomain:
			score += 10000
		case models.RoleExecution:
			score += 8000
		case models.RoleAgent:
			score += 7000
		case models.RoleProvider:
			score += 6000
		case models.RolePersistence:
			score += 5000
		case models.RoleInfrastructure:
			score += 4000
		case models.RoleUtility:
			score -= 5000 // Penalty
		case models.RoleConfig:
			score -= 8000 // Penalty
		case models.RoleTest:
			score -= 20000 // Heavy penalty
		case models.RoleGenerated:
			score -= 25000
		}

		f.Score = int64(score + mentalModelScore)
	}

	// 5. Sort files by score descending
	sort.Slice(repo.Files, func(i, j int) bool {
		return repo.Files[i].Score > repo.Files[j].Score
	})
}

// Generator creates structured learning paths from analyzed repository data.
type Generator struct{}

// GenerateUnits produces a sequence of LearningUnits based on priority and dependencies.
func (g *Generator) GenerateUnits(repo *models.Repository) []models.LearningUnit {
	if len(repo.Concepts) == 0 {
		return nil
	}

	// 1. Calculate Priority for each concept
	conceptMap := make(map[string]*models.Concept)
	unlockCount := make(map[string]int)
	for _, edge := range repo.ConceptEdges {
		unlockCount[edge.To]++
	}

	priorities := make(map[string]float64)
	for i := range repo.Concepts {
		c := &repo.Concepts[i]
		conceptMap[c.ID] = c

		// LearningRanking (Human Bias)
		priority := c.Importance
		
		switch c.Type {
		case models.CoreAbstraction:
			priority += 500 // Mental Model Root Boost
		case models.EntryAPI:
			priority += 300
		case models.SupportingType:
			priority -= 300
		case models.UtilityType:
			priority -= 500
		}

		// Weight Public API significantly (Evidence of intended learning targets)
		priority += c.Factors.PublicAPIWeight * 0.5

		switch c.Role {
		case models.RoleFoundational:
			priority += 200
		case models.RoleCore:
			priority += 100
		case models.RolePeripheral:
			priority -= 100
		}

		// Penalty for being deep in the dependency graph (Internal complexity)
		priority -= float64(len(c.PrerequisiteIDs)) * 50

		c.LearningPriority = priority
		priorities[c.ID] = priority
	}

	// 2. Ordered IDs based on Priority
	type conceptPriority struct {
		id       string
		priority float64
	}
	var sortedPriorities []conceptPriority
	for id, p := range priorities {
		sortedPriorities = append(sortedPriorities, conceptPriority{id, p})
	}
	sort.Slice(sortedPriorities, func(i, j int) bool {
		return sortedPriorities[i].priority > sortedPriorities[j].priority
	})

	// 3. Respect Prerequisites (Topological Pass)
	var orderedIDs []string
	visited := make(map[string]bool)
	inProgress := make(map[string]bool)
	
	var visit func(id string)
	visit = func(id string) {
		if inProgress[id] || visited[id] {
			return
		}
		inProgress[id] = true
		c := conceptMap[id]
		// Only strictly required prerequisites that are ALSO core/foundational
		// AND not just utility types that happen to be dependencies.
		for _, preID := range c.PrerequisiteIDs {
			pre := conceptMap[preID]
			if pre != nil && (pre.Role == models.RoleFoundational || pre.Role == models.RoleCore) {
				if pre.Type != models.UtilityType && pre.Type != models.SupportingType {
					visit(preID)
				}
			}
		}
		visited[id] = true
		inProgress[id] = false
		orderedIDs = append(orderedIDs, id)
	}

	for _, cp := range sortedPriorities {
		visit(cp.id)
	}

	// Populate Learning Ranking in Repo
	repo.Rankings.LearningRanking = orderedIDs

	// 4. Build Units and Calculate Coverage
	units := make([]models.LearningUnit, 0, len(orderedIDs))
	totalSymbols := 0
	for _, f := range repo.Files {
		totalSymbols += len(f.Symbols)
	}

	coveredSymbols := make(map[string]bool)
	coveredConcepts := make(map[string]bool)
	totalImportance := 0.0
	for _, c := range repo.Concepts {
		totalImportance += c.Importance
	}

	for i, id := range orderedIDs {
		c := conceptMap[id]
		
		time := len(c.SymbolIDs) * 2
		if time < 5 { time = 5 }
		if time > 20 { time = 20 }

		gain := 0.0
		if totalImportance > 0 {
			gain = (c.Importance / totalImportance) * 100
		}

		for _, symID := range c.SymbolIDs {
			coveredSymbols[symID] = true
		}
		coveredConcepts[id] = true

		coverage := models.Coverage{
			SymbolsCovered:  len(coveredSymbols),
			TotalSymbols:    totalSymbols,
			ConceptsCovered: len(coveredConcepts),
			TotalConcepts:   len(repo.Concepts),
		}
		if totalSymbols > 0 {
			coverage.SymbolPercentage = (float64(coverage.SymbolsCovered) / float64(totalSymbols)) * 100
		}
		if len(repo.Concepts) > 0 {
			coverage.ConceptPercentage = (float64(coverage.ConceptsCovered) / float64(len(repo.Concepts))) * 100
		}

		units = append(units, models.LearningUnit{
			Order:                   i + 1,
			ID:                      c.ID,
			Name:                    c.Name,
			SymbolIDs:               c.SymbolIDs,
			PrerequisiteIDs:         c.PrerequisiteIDs,
			Purpose:                 c.Summary.Purpose,
			LearningObjective:       c.Summary.LearningObjective,
			EstimatedTimeMinutes:    time,
			KnowledgeGainPercentage: gain,
			Coverage:                coverage,
			Slice:                   g.extractSlice(repo, c),
			Importance:              c.Importance,
		})
	}

	return units
}

// extractSlice identifies the relevant code regions for a concept across all files.
func (g *Generator) extractSlice(repo *models.Repository, concept *models.Concept) models.CodeSlice {
	slice := models.CodeSlice{
		ConceptID: concept.ID,
		Files:     make([]models.FileSlice, 0),
	}

	// 1. Group symbols by file
	fileToSyms := make(map[string][]models.Symbol)
	idToFile := make(map[string]*models.File)
	for _, f := range repo.Files {
		for _, sym := range f.Symbols {
			idToFile[sym.ID] = f
			for _, cid := range concept.SymbolIDs {
				if sym.ID == cid {
					fileToSyms[f.Path] = append(fileToSyms[f.Path], sym)
					break
				}
			}
		}
	}

	totalHidden := 0
	for path, syms := range fileToSyms {
		fileSlice := models.FileSlice{
			FilePath:   path,
			Ranges:     make([]models.LineRange, 0),
			Highlights: make([]models.Highlight, 0),
		}

		// 2. Collect ranges and highlights
		for _, sym := range syms {
			// Highlight the declaration (first 5 lines or until end)
			hEnd := sym.StartLine + 4
			if hEnd > sym.EndLine {
				hEnd = sym.EndLine
			}

			if sym.Kind == models.StructSymbol || sym.Kind == models.InterfaceSymbol {
				fileSlice.Ranges = append(fileSlice.Ranges, models.LineRange{
					Start: sym.StartLine,
					End:   sym.EndLine,
				})
				fileSlice.Highlights = append(fileSlice.Highlights, models.Highlight{
					Start:  sym.StartLine,
					End:    hEnd,
					Reason: fmt.Sprintf("Definition of %s %s", sym.Kind, sym.Name),
				})
			} else {
				// Show function signature
				fileSlice.Ranges = append(fileSlice.Ranges, models.LineRange{
					Start: sym.StartLine,
					End:   hEnd,
				})
				fileSlice.Highlights = append(fileSlice.Highlights, models.Highlight{
					Start:  sym.StartLine,
					End:    sym.StartLine,
					Reason: "Public API / Entrypoint",
				})

				// Apply Line Classifier Blocks
				f := idToFile[sym.ID]
				for _, block := range sym.Blocks {
					if block.Visibility == models.VisibilityCritical || (block.Visibility == models.VisibilityUseful && (sym.Score > 500 || f.Role == models.RoleEntrypoint)) {
						fileSlice.Ranges = append(fileSlice.Ranges, models.LineRange{
							Start: block.StartLine,
							End:   block.EndLine,
						})
						
						if block.Visibility == models.VisibilityCritical {
							fileSlice.Highlights = append(fileSlice.Highlights, models.Highlight{
								Start:  block.StartLine,
								End:    block.EndLine,
								Reason: block.Reason,
							})
						}
					}
				}

				// Always ensure the final closing brace is included
				fileSlice.Ranges = append(fileSlice.Ranges, models.LineRange{
					Start: sym.EndLine,
					End:   sym.EndLine,
				})
			}
		}

		// 3. Sort and Merge ranges
		sort.Slice(fileSlice.Ranges, func(i, j int) bool {
			return fileSlice.Ranges[i].Start < fileSlice.Ranges[j].Start
		})

		merged := make([]models.LineRange, 0)
		if len(fileSlice.Ranges) > 0 {
			curr := fileSlice.Ranges[0]
			for i := 1; i < len(fileSlice.Ranges); i++ {
				next := fileSlice.Ranges[i]
				if next.Start <= curr.End+1 { // Merge adjacent or overlapping
					if next.End > curr.End {
						curr.End = next.End
					}
				} else {
					merged = append(merged, curr)
					curr = next
				}
			}
			merged = append(merged, curr)
		}
		fileSlice.Ranges = merged

		// 4. Calculate hidden lines in this file
		shownLines := 0
		for _, r := range merged {
			shownLines += (r.End - r.Start + 1)
		}
		f := idToFile[syms[0].ID]
		totalHidden += (f.TotalLines - shownLines)

		slice.Files = append(slice.Files, fileSlice)
	}

	slice.HiddenLinesCount = totalHidden
	return slice
}

// Generate produces a list of LearningSteps (Legacy file-based path).
func (g *Generator) Generate(repo *models.Repository) []models.LearningStep {
	steps := make([]models.LearningStep, 0, len(repo.Files))

	for i, f := range repo.Files {
		var purpose, learningObjective string

		// Extract important ranges for this file
		fileSlice := models.FileSlice{
		        FilePath: f.Path,
		}

		// Sort symbols by start line to merge ranges
		sort.Slice(f.Symbols, func(i, j int) bool {
			return f.Symbols[i].StartLine < f.Symbols[j].StartLine
		})

		// Determine the single most important symbol in the file for full display
		var bestSymID string
		maxScore := -1.0
		for _, sym := range f.Symbols {
			if sym.Score > maxScore {
				maxScore = sym.Score
				bestSymID = sym.ID
			}
		}

		for _, sym := range f.Symbols {
			// Refined importance logic
			importanceLevel := 0 // 0: ignore, 1: signature only, 2: full

			if sym.Kind == models.StructSymbol || sym.Kind == models.InterfaceSymbol {
				// Show data structures if they are somewhat relevant
				if sym.Score > 400 || sym.ID == bestSymID {
					importanceLevel = 2
				} else {
					importanceLevel = 1
				}
			} else if sym.Kind == models.FunctionSymbol || sym.Kind == models.MethodSymbol {
				// Only show full body for the absolute best symbol (if it's very high score) or main
				if (sym.ID == bestSymID && sym.Score > 2000) || (f.Role == models.RoleEntrypoint && sym.Name == "main") {
					importanceLevel = 2
				} else if sym.Score > 500 || strings.HasPrefix(sym.Name, "New") {
					// Show signature for other important functions/constructors
					importanceLevel = 1
				}
			}

			if importanceLevel == 2 {
				fileSlice.Ranges = append(fileSlice.Ranges, models.LineRange{
					Start: sym.StartLine,
					End:   sym.EndLine,
				})
			} else if importanceLevel == 1 {
				// Show just the signature (first few lines)
				end := sym.StartLine + 2
				if end > sym.EndLine {
					end = sym.EndLine
				}
				fileSlice.Ranges = append(fileSlice.Ranges, models.LineRange{
					Start: sym.StartLine,
					End:   end,
				})
			}
		}

		// Merge overlapping/adjacent ranges
		if len(fileSlice.Ranges) > 0 {
			merged := make([]models.LineRange, 0)
			curr := fileSlice.Ranges[0]
			for j := 1; j < len(fileSlice.Ranges); j++ {
				next := fileSlice.Ranges[j]
				if next.Start <= curr.End+1 {
					if next.End > curr.End {
						curr.End = next.End
					}
				} else {
					merged = append(merged, curr)
					curr = next
				}
			}
			merged = append(merged, curr)
			fileSlice.Ranges = merged
		}

		// Identify Key Symbols (Top 3 exported)
		sort.Slice(f.Symbols, func(i, j int) bool {
			return f.Symbols[i].Score > f.Symbols[j].Score
		})
		keySymbols := make([]string, 0)
		for _, sym := range f.Symbols {
			if len(sym.Name) > 0 && sym.Name[0] >= 'A' && sym.Name[0] <= 'Z' {
				keySymbols = append(keySymbols, sym.Name)
				if len(keySymbols) >= 3 {
					break
				}
			}
		}

		// Identify Unlocks (Directly referenced files that appear later)
		// This is a simplified version: just look at what this file imports/references
		unlocks := make([]string, 0)
		// We'll use the symbol reference graph to see what files are 'unlocked'
		seenUnlocks := make(map[string]bool)
		for _, sym := range f.Symbols {
			for _, edge := range repo.SymbolEdges {
				if edge.From == sym.ID {
					// Find which file contains edge.To
					for _, targetFile := range repo.Files {
						for _, targetSym := range targetFile.Symbols {
							if targetSym.ID == edge.To {
								targetRel := filepath.Base(targetFile.Path)
								if targetFile.Path != f.Path && !seenUnlocks[targetRel] {
									unlocks = append(unlocks, targetRel)
									seenUnlocks[targetRel] = true
								}
								break
							}
						}
						if seenUnlocks[filepath.Base(targetFile.Path)] {
							break
						}
					}
				}
				if len(unlocks) >= 3 {
					break
				}
			}
			if len(unlocks) >= 3 {
				break
			}
		}

		steps = append(steps, models.LearningStep{
			Order:             i + 1,
			File:              f.Path,
			Purpose:           purpose,
			LearningObjective: learningObjective,
			KeySymbols:        keySymbols,
			Unlocks:           unlocks,
			Score:             float64(f.Score),
			Slice:             fileSlice,
		})
	}

	return steps
}
