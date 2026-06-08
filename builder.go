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

				if count >= 30 {
					subIndex := count / 30
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

		internalEdges := 0
		symSet := make(map[string]bool)
		for _, id := range c.SymbolIDs {
			symSet[id] = true
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

		concepts = append(concepts, *c)
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

			// Boilerplate Penalties
			boilerplate := map[string]bool{
				"Len": true, "Less": true, "Swap": true, "String": true, "Error": true,
			}
			if boilerplate[sym.Name] {
				sym.Score -= 40
			}
		}
	}

	// 2. Apply Centrality (Incoming References with typed weights)
	primitives := map[string]bool{
		"string": true, "int": true, "int64": true, "bool": true, "error": true,
		"interface{}": true, "any": true, "float64": true, "byte": true, "rune": true,
	}

	for _, edge := range repo.SymbolEdges {
		if primitives[strings.ToLower(edge.To)] {
			continue
		}
		if target, ok := idToSymbol[edge.To]; ok {
			var weight float64
			switch edge.Type {
			case models.RefImplement:
				weight = 100
			case models.RefEmbed:
				weight = 80
			case models.RefField:
				weight = 50
			case models.RefConstruct:
				weight = 40
			case models.RefReturn:
				weight = 20
			case models.RefParameter:
				weight = 10
			case models.RefCall:
				weight = 10
			default:
				weight = 10
			}
			target.Score += weight
		}
	}

	// 2.5 Entrypoint Reachability
	// Find distance from entrypoint functions to all other symbols
	distances := make(map[string]int)
	queue := []string{}

	// Identify entrypoint symbols
	for _, f := range repo.Files {
		if f.Role == models.RoleEntrypoint {
			for _, sym := range f.Symbols {
				if sym.Kind == models.FunctionSymbol && sym.Name == "main" {
					distances[sym.ID] = 0
					queue = append(queue, sym.ID)
				}
			}
		}
	}

	// Adjacency list for BFS (From -> To)
	adj := make(map[string][]string)
	for _, edge := range repo.SymbolEdges {
		adj[edge.From] = append(adj[edge.From], edge.To)
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		dist := distances[curr]

		for _, neighbor := range adj[curr] {
			if _, seen := distances[neighbor]; !seen {
				distances[neighbor] = dist + 1
				queue = append(queue, neighbor)
				
				// Apply reachability bonus
				if target, ok := idToSymbol[neighbor]; ok {
					bonus := 100.0 / float64(dist+1)
					target.Score += bonus
				}
			}
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

// Generator creates structured learning paths from analyzed repository data.
type Generator struct{}

// GenerateUnits produces a sequence of LearningUnits based on detected concepts and dependencies.
func (g *Generator) GenerateUnits(repo *models.Repository) []models.LearningUnit {
	if len(repo.Concepts) == 0 {
		return nil
	}

	// 1. Map concepts for easy lookup
	conceptMap := make(map[string]models.Concept)
	for _, c := range repo.Concepts {
		conceptMap[c.ID] = c
	}

	// 2. Topological Sort (Kahn's Algorithm variant)
	// We want to order units such that prerequisites come first.
	var orderedIDs []string
	visited := make(map[string]bool)
	temp := make(map[string]bool)

	var visit func(id string)
	visit = func(id string) {
		if temp[id] {
			// Cycle detected or already in progress - skip for now
			return
		}
		if !visited[id] {
			temp[id] = true
			c := conceptMap[id]
			for _, preID := range c.PrerequisiteIDs {
				if _, exists := conceptMap[preID]; exists {
					visit(preID)
				}
			}
			visited[id] = true
			temp[id] = false
			orderedIDs = append(orderedIDs, id)
		}
	}

	// Start with highest importance concepts but respect their prerequisites
	for _, c := range repo.Concepts {
		visit(c.ID)
	}

	// 3. Build Units and Calculate Coverage
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

		reason := "Core architectural component"
		if c.Importance > 1000 {
			reason = "Foundational system element"
		} else if strings.Contains(strings.ToLower(c.Name), "test") {
			reason = "Verification and usage examples"
		}

		units = append(units, models.LearningUnit{
			Order:                   i + 1,
			ID:                      c.ID,
			Name:                    c.Name,
			SymbolIDs:               c.SymbolIDs,
			PrerequisiteIDs:         c.PrerequisiteIDs,
			Reason:                  reason,
			EstimatedTimeMinutes:    time,
			KnowledgeGainPercentage: gain,
			Coverage:                coverage,
			Importance:              c.Importance,
		})
	}

	return units
}

// Generate produces a list of LearningSteps (Legacy file-based path).
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
