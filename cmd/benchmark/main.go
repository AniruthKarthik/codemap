package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/AniruthKarthik/codemap"
)

type BenchmarkExpectation struct {
	RepoUrl     string
	TopConcepts []string
}

var benchmarks = []BenchmarkExpectation{
	{
		RepoUrl: "https://github.com/spf13/cobra",
		TopConcepts: []string{
			"Command",
			"Execute",
			"AddCommand",
			"Flags",
		},
	},
	{
		RepoUrl: "https://github.com/gin-gonic/gin",
		TopConcepts: []string{
			"Engine",
			"RouterGroup",
			"Context",
		},
	},
	{
		RepoUrl: "https://github.com/gohugoio/hugo",
		TopConcepts: []string{
			"Site",
			"Page",
			"Build",
		},
	},
}

type BenchmarkResult struct {
	ExpectedName string
	Found        bool
	Position     int
	Score        float64
}

func main() {
	flag.Parse()

	fmt.Println("=== Codemap Ground Truth Validation ===")

	totalAccuracy := 0.0
	validRepos := 0

	for _, b := range benchmarks {
		repoName := strings.Split(b.RepoUrl, "/")[len(strings.Split(b.RepoUrl, "/"))-1]
		tmpDir := "/tmp/codemap_benchmark_" + repoName
		
		fmt.Printf("\nRepo: %s\n", b.RepoUrl)
		
		repo, err := codemap.BuildRepository(tmpDir)
		if err != nil {
			fmt.Printf("   [SKIP] Error: %v (Ensure repo is cloned to %s)\n", err, tmpDir)
			continue
		}

		generator := &codemap.Generator{}
		units := generator.GenerateUnits(repo)

		results := make([]BenchmarkResult, len(b.TopConcepts))
		for i, expected := range b.TopConcepts {
			results[i] = BenchmarkResult{ExpectedName: expected, Found: false, Position: -1}
		}

		fmt.Println("   Matches in Top 10:")
		for i := 0; i < len(units) && i < 10; i++ {
			unit := units[i]
			match := ""
			
			// Collect all symbol names in this unit
			unitSymbolNames := make(map[string]bool)
			for _, symID := range unit.SymbolIDs {
				parts := strings.Split(symID, ".")
				name := parts[len(parts)-1]
				unitSymbolNames[strings.ToLower(name)] = true
			}

			for j := range results {
				expectedLower := strings.ToLower(results[j].ExpectedName)
				foundInSymbols := unitSymbolNames[expectedLower]
				foundInName := strings.Contains(strings.ToLower(unit.Name), expectedLower)

				if !results[j].Found && (foundInName || foundInSymbols) {
					results[j].Found = true
					results[j].Position = i + 1
					
					if results[j].Position <= 3 {
						results[j].Score = 100
					} else if results[j].Position <= 6 {
						results[j].Score = 80
					} else {
						results[j].Score = 50
					}
					
					if match == "" {
						match = "[MATCH: " + results[j].ExpectedName
					} else {
						match += ", " + results[j].ExpectedName
					}
				}
			}
			if match != "" { match += "]" }
			fmt.Printf("     %2d. %-25s %s\n", i+1, unit.Name, match)

			// Find concept in repo to get factors
			for _, c := range repo.Concepts {
				if c.ID == unit.ID {
					fmt.Printf("         Type: %-15s Imp: %6.0f Prio: %6.0f Factors: Cent=%.0f, Unlocks=%.0f, API=%.0f, Size=%.0f\n",
						c.Type, c.Importance, unit.Importance, c.Factors.Centrality, c.Factors.DependencyUnlocks,
						c.Factors.PublicAPIWeight, c.Factors.ConceptSize)
					break
				}
			}
		}

		repoAccuracy := 0.0
		foundCount := 0
		for _, res := range results {
			if res.Found {
				foundCount++
				repoAccuracy += res.Score
			}
		}
		repoAccuracy = repoAccuracy / float64(len(b.TopConcepts))

		fmt.Printf("   Accuracy: %.1f%% (%d/%d expected found)\n", repoAccuracy, foundCount, len(b.TopConcepts))
		
		totalAccuracy += repoAccuracy
		validRepos++
	}

	if validRepos > 0 {
		fmt.Printf("\nOVERALL BENCHMARK SCORE: %.1f%%\n", totalAccuracy/float64(validRepos))
	}
}
