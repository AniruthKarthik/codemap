package parser

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/AniruthKarthik/codemap/internal/models"
)

func TestGoParser_Parse(t *testing.T) {
	tests := []struct {
		name              string
		content           string
		expectedPackage   string
		expectedImports   []string
		expectedFunctions []models.Function
	}{
		{
			name:              "main package with no imports",
			content:           "package main\n\nfunc main() {}\n",
			expectedPackage:   "package main",
			expectedImports:   []string{},
			expectedFunctions: []models.Function{{Name: "main", StartLine: 3, EndLine: 3, Exported: false}},
		},
		{
			name: "router package with imports and functions",
			content: `package router

import (
	"fmt"
	"net/http"
)

func Login() {}

func logout() {}

type Router struct{}
`,
			expectedPackage: "package router",
			expectedImports: []string{"fmt", "net/http"},
			expectedFunctions: []models.Function{
				{Name: "Login", StartLine: 8, EndLine: 8, Exported: true},
				{Name: "logout", StartLine: 10, EndLine: 10, Exported: false},
			},
		},
		{
			name: "single line import",
			content: `package main
import "os"
func main() {}
`,
			expectedPackage:   "package main",
			expectedImports:   []string{"os"},
			expectedFunctions: []models.Function{{Name: "main", StartLine: 3, EndLine: 3, Exported: false}},
		},
	}

	p := &GoParser{}
	tmpDir, err := os.MkdirTemp("", "parser_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(tmpDir, tt.name+".go")
			err := os.WriteFile(path, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			file, err := p.Parse(path)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if file.Package != tt.expectedPackage {
				t.Errorf("got package %q, want %q", file.Package, tt.expectedPackage)
			}

			if !reflect.DeepEqual(file.Imports, tt.expectedImports) {
				t.Errorf("got imports %v, want %v", file.Imports, tt.expectedImports)
			}

			if !reflect.DeepEqual(file.Functions, tt.expectedFunctions) {
				t.Errorf("got functions %+v, want %+v", file.Functions, tt.expectedFunctions)
			}
		})
	}
}
