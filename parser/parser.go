package parser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/AniruthKarthik/codemap/internal/models"
)

type Parser interface {
	Parse(path string) (*models.File, error)
}

type GoParser struct{}

func (p *GoParser) Parse(path string) (*models.File, error) {
	fset := token.NewFileSet()
	// Using parser.ParseComments to ensure we get as much info as needed, 
	// though parser.AllErrors or 0 might suffice for just declarations.
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
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
