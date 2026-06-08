package main

import (
        "bufio"
        "encoding/json"
        "flag"
        "fmt"
        "log"
        "net/http"
        "os"
        "path/filepath"
        "strings"

        "github.com/AniruthKarthik/codemap"
        "github.com/AniruthKarthik/codemap/internal/models"
)

func main() {
        if len(os.Args) > 1 && os.Args[1] == "serve" {
                runServe()
                return
        }

        fs := flag.NewFlagSet("codemap", flag.ExitOnError)
        topN := fs.Int("top", 0, "Output only the first N recommended files")
        outputJSON := fs.Bool("json", false, "Output results in JSON format")
        debug := fs.Bool("debug", false, "Enable debug mode for detailed analysis")
        noCode := fs.Bool("nocode", false, "Do not print important lines of code")
        full := fs.Bool("full", false, "Show complete file contents with highlights")

        fs.Usage = func() {
                fmt.Fprintf(os.Stderr, "Usage: codemap <repository-path> [flags]\n\n")
                fmt.Fprintf(os.Stderr, "Example:\n  codemap .\n  codemap ~/projects/cobra --top 20\n\n")
                fmt.Fprintf(os.Stderr, "Flags:\n")
                fs.PrintDefaults()
        }

        // Custom parsing to handle: codemap <path> <flags>
        var repoPath string
        var flagArgs []string

        if len(os.Args) > 1 {
                if strings.HasPrefix(os.Args[1], "-") {
                        // First arg is a flag: codemap <flags> <path>
                        fs.Parse(os.Args[1:])
                        args := fs.Args()
                        if len(args) > 0 {
                                repoPath = args[0]
                        }
                } else {
                        // First arg is the path: codemap <path> <flags>
                        repoPath = os.Args[1]
                        flagArgs = os.Args[2:]
                        fs.Parse(flagArgs)
                }
        }

        if repoPath == "" {
                fmt.Fprintln(os.Stderr, "Error: Repository path is required.")
                fs.Usage()
                os.Exit(1)
        }

        // 1. Validate path
        info, err := os.Stat(repoPath)
        if os.IsNotExist(err) {
                fmt.Fprintf(os.Stderr, "Error: Repository path %q does not exist.\n", repoPath)
                os.Exit(1)
        }
        if !info.IsDir() {
                fmt.Fprintf(os.Stderr, "Error: Path %q is not a directory.\n", repoPath)
                os.Exit(1)
        }

        // 2. Build Repository (Scan, Analyze, Rank)
        repo, err := codemap.BuildRepository(repoPath)
        if err != nil {
                fmt.Fprintf(os.Stderr, "Error: Analysis failed: %v\n", err)
                os.Exit(1)
        }

        if len(repo.Files) == 0 {
                fmt.Fprintf(os.Stderr, "Error: Repository contains no supported source files.\n")
                os.Exit(1)
        }

        // 3. Generate Reading Order
        generator := &codemap.Generator{}
        steps := generator.Generate(repo)

        // Apply Top-N
        if *topN > 0 && *topN < len(steps) {
                steps = steps[:*topN]
        }

        // 4. Output
        if *outputJSON {
                printJSON(repo, steps, repoPath, *noCode, *full)
        } else {
                printTerminal(repo, steps, repoPath, *debug, *noCode, *full)
        }
}

func runServe() {
        serveFs := flag.NewFlagSet("serve", flag.ExitOnError)
        port := serveFs.String("port", "8080", "Port to run the server on")

        var repoPath string
        if len(os.Args) > 2 {
                if !strings.HasPrefix(os.Args[2], "-") {
                        repoPath = os.Args[2]
                        serveFs.Parse(os.Args[3:])
                } else {
                        serveFs.Parse(os.Args[2:])
                }
        }

        if repoPath == "" {
                repoPath = "."
        }

        absPath, err := filepath.Abs(repoPath)
        if err != nil {
                log.Fatalf("Error: Invalid path: %v", err)
        }

        fmt.Printf("Starting Codemap Server at http://localhost:%s\n", *port)
        fmt.Printf("Analyzing repository: %s\n", absPath)

        // Middleware for CORS
        corsMiddleware := func(h http.HandlerFunc) http.HandlerFunc {
                return func(w http.ResponseWriter, r *http.Request) {
                        w.Header().Set("Access-Control-Allow-Origin", "*")
                        w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
                        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
                        
                        if r.Method == "OPTIONS" {
                                w.WriteHeader(http.StatusOK)
                                return
                        }
                        h(w, r)
                }
        }

        // API: Get Analysis
        http.HandleFunc("/api/analysis", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
                target := r.URL.Query().Get("path")
                if target == "" {
                        target = absPath
                }

                repo, err := codemap.BuildRepository(target)
                if err != nil {
                        http.Error(w, err.Error(), http.StatusInternalServerError)
                        return
                }
                generator := &codemap.Generator{}
                steps := generator.Generate(repo)

                data := struct {
                        Repository string                `json:"repository"`
                        Path       string                `json:"path"`
                        Steps      []models.LearningStep `json:"steps"`
                }{
                        Repository: filepath.Base(target),
                        Path:       target,
                        Steps:      steps,
                }

                w.Header().Set("Content-Type", "application/json")
                json.NewEncoder(w).Encode(data)
        }))

        // API: List Directories for File Browser
        http.HandleFunc("/api/ls", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
                path := r.URL.Query().Get("path")
                if path == "" {
                        path, _ = os.UserHomeDir()
                }

                entries, err := os.ReadDir(path)
                if err != nil {
                        http.Error(w, err.Error(), http.StatusInternalServerError)
                        return
                }

                type Entry struct {
                        Name  string `json:"name"`
                        IsDir bool   `json:"isDir"`
                        Path  string `json:"path"`
                }

                var result []Entry
                // Add parent directory option
                parent := filepath.Dir(path)
                if parent != path {
                        result = append(result, Entry{Name: "..", IsDir: true, Path: parent})
                }

                for _, e := range entries {
                        // Skip hidden files/dirs in the browser for clarity
                        if strings.HasPrefix(e.Name(), ".") && e.Name() != "." && e.Name() != ".." {
                                continue
                        }
                        result = append(result, Entry{
                                Name:  e.Name(),
                                IsDir: e.IsDir(),
                                Path:  filepath.Join(path, e.Name()),
                        })
                }

                w.Header().Set("Content-Type", "application/json")
                json.NewEncoder(w).Encode(result)
        }))

        // API: Get Home Directory
        http.HandleFunc("/api/home", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
                home, _ := os.UserHomeDir()
                fmt.Fprintf(w, "%s", home)
        }))

        // API: Get File Content
        http.HandleFunc("/api/file", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
                filePath := r.URL.Query().Get("path")
                if filePath == "" {
                        http.Error(w, "path is required", http.StatusBadRequest)
                        return
                }

                // Security check: ensure path is absolute and exists
                if !filepath.IsAbs(filePath) {
                        http.Error(w, "absolute path required", http.StatusBadRequest)
                        return
                }

                content, err := os.ReadFile(filePath)
                if err != nil {
                        http.Error(w, err.Error(), http.StatusNotFound)
                        return
                }
                w.Header().Set("Content-Type", "text/plain")
                w.Write(content)
        }))

        log.Fatal(http.ListenAndServe(":"+*port, nil))
}

type JSONOutput struct {
	Repository string           `json:"repository"`
	Files      []JSONFileOutput `json:"files"`
}

type JSONFileOutput struct {
        Rank              int      `json:"rank"`
        Path              string   `json:"path"`
        Purpose           string   `json:"purpose"`
        LearningObjective string   `json:"learning_objective"`
        KeySymbols        []string `json:"key_symbols,omitempty"`
        Unlocks           []string `json:"unlocks,omitempty"`
        Code              []string `json:"code,omitempty"`
}

func printJSON(repo *models.Repository, steps []models.LearningStep, repoPath string, noCode bool, full bool) {
        absPath, _ := filepath.Abs(repoPath)
        output := JSONOutput{
                Repository: filepath.Base(absPath),
                Files:      make([]JSONFileOutput, 0, len(steps)),
        }

        for _, step := range steps {
                relPath, _ := filepath.Rel(repoPath, step.File)

                var lines []string
                if !noCode {
                        lines = getImportantLines(step.File, step.Slice.Ranges, full)
                }

                output.Files = append(output.Files, JSONFileOutput{
                        Rank:              step.Order,
                        Path:              relPath,
                        Purpose:           step.Purpose,
                        LearningObjective: step.LearningObjective,
                        KeySymbols:        step.KeySymbols,
                        Unlocks:           step.Unlocks,
                        Code:              lines,
                })
        }

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func printTerminal(repo *models.Repository, steps []models.LearningStep, repoPath string, debug bool, noCode bool, full bool) {
        absPath, _ := filepath.Abs(repoPath)
        fmt.Printf("Repository: %s\n\n", filepath.Base(absPath))
        fmt.Printf("Files analyzed: %d\n\n", len(repo.Files))

        if debug {
                printDebug(repo)
        }

        fmt.Println("Recommended Reading Order")
        for i, step := range steps {
                relPath, _ := filepath.Rel(repoPath, step.File)
                fmt.Printf("\n%d. %s\n", i+1, relPath)
                fmt.Printf("   Purpose:   %s\n", step.Purpose)
                fmt.Printf("   Objective: %s\n", step.LearningObjective)

                if len(step.KeySymbols) > 0 {
                        fmt.Printf("   Symbols:   %s\n", strings.Join(step.KeySymbols, ", "))
                }
                if len(step.Unlocks) > 0 {
                        fmt.Printf("   Unlocks:   %s\n", strings.Join(step.Unlocks, ", "))
                }

                if !noCode {
                        fmt.Println("\n   Code Analysis:")
                        lines := getImportantLines(step.File, step.Slice.Ranges, full)
                        for _, line := range lines {
                                fmt.Printf("      %s\n", line)
                        }
                }
        }
}

func getImportantLines(filePath string, ranges []models.LineRange, full bool) []string {
        file, err := os.Open(filePath)
        if err != nil {
                return []string{"Error: Could not open file"}
        }
        defer file.Close()

        var result []string
        scanner := bufio.NewScanner(file)
        currentLine := 1
        rangeIdx := 0

        for scanner.Scan() {
                isImportant := false
                if rangeIdx < len(ranges) {
                        if currentLine >= ranges[rangeIdx].Start && currentLine <= ranges[rangeIdx].End {
                                isImportant = true
                        }
                }

                if isImportant {
                        result = append(result, fmt.Sprintf("%4d | %s", currentLine, scanner.Text()))
                } else if full {
                        result = append(result, fmt.Sprintf("     : %s", scanner.Text()))
                } else {
                        // Not important and not full mode: skipping logic
                        if currentLine == 1 && rangeIdx < len(ranges) && currentLine < ranges[rangeIdx].Start {
                                hiddenCount := ranges[rangeIdx].Start - 1
                                if hiddenCount > 0 {
                                        result = append(result, fmt.Sprintf("     | ... (%d lines hidden)", hiddenCount))
                                }
                        }
                }

                // Advance range index if we reached the end of current range
                if rangeIdx < len(ranges) && currentLine == ranges[rangeIdx].End {
                        rangeIdx++
                        if !full && rangeIdx < len(ranges) {
                                hiddenCount := ranges[rangeIdx].Start - currentLine - 1
                                if hiddenCount > 0 {
                                        result = append(result, fmt.Sprintf("     | ... (%d lines hidden)", hiddenCount))
                                }
                        }
                }

                // If not in full mode and not important, skip to next important line or end of file
                if !full && !isImportant {
                        if rangeIdx < len(ranges) {
                                if currentLine < ranges[rangeIdx].Start {
                                        currentLine++
                                        continue
                                }
                        } else {
                                // No more ranges, count remaining lines and break
                                hiddenRemaining := 0
                                for scanner.Scan() {
                                        hiddenRemaining++
                                }
                                if hiddenRemaining > 0 {
                                        result = append(result, fmt.Sprintf("     | ... (%d lines hidden)", hiddenRemaining))
                                }
                                break
                        }
                }

                currentLine++
        }

        return result
}

func printDebug(repo *models.Repository) {
	fmt.Println("--- DEBUG INFO ---")

	// Entrypoints
	detector := &codemap.EntrypointDetector{}
	entrypoints := detector.Find(repo)
	fmt.Printf("Entrypoints detected: %d\n", len(entrypoints))
	for _, e := range entrypoints {
		fmt.Printf("  - %s\n", e)
	}

	// Concepts
	fmt.Printf("Concepts detected: %d\n", len(repo.Concepts))

	// Symbols
	totalSymbols := 0
	for _, f := range repo.Files {
		totalSymbols += len(f.Symbols)
	}
	fmt.Printf("Symbol count: %d\n", totalSymbols)

	// Graph Statistics
	fmt.Printf("Reference graph statistics: %d edges\n", len(repo.SymbolEdges))

	// File classification and ranking factors
	fmt.Println("\nFile Details (Top 5):")
	for i := 0; i < 5 && i < len(repo.Files); i++ {
		f := repo.Files[i]
		fmt.Printf("  - %s\n", filepath.Base(f.Path))
		fmt.Printf("    Score: %d, Role: %s\n", f.Score, f.Role)
	}
	fmt.Println("------------------")
	}
