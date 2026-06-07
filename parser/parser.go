package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/AniruthKarthik/codemap/internal/models"
)

// Parser defines the behavior for extracting metadata from a Go source file.
type Parser interface {
	Parse(path string) (*models.File, error)
}

// GoParser is an implementation of the Parser interface for Go source files.
type GoParser struct{}

// NewGoParser creates a new GoParser instance.
func NewGoParser() *GoParser {
	return &GoParser{}
}

// Parse extracts package information, imports, and function declarations from a Go file.
func (p *GoParser) Parse(path string) (*models.File, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file %q: %w", path, err)
	}

	imports := make([]string, 0, len(f.Imports))
	for _, spec := range f.Imports {
		path := strings.Trim(spec.Path.Value, "\"")
		imports = append(imports, path)
	}

	var functions []models.Function
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			start := fset.Position(fn.Pos()).Line
			end := fset.Position(fn.End()).Line

			functions = append(functions, models.Function{
				Name:      fn.Name.Name,
				StartLine: start,
				EndLine:   end,
				Exported:  fn.Name.IsExported(),
			})
		}
	}

	return &models.File{
		Path:      path,
		Package:   "package " + f.Name.Name,
		Imports:   imports,
		Functions: functions,
	}, nil
}
