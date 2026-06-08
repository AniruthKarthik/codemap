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

// Parse extracts package information, imports, symbols (structs, interfaces, methods, functions) from a Go file.
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
	var symbols []models.Symbol
	pkgName := f.Name.Name

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			start := fset.Position(d.Pos()).Line
			end := fset.Position(d.End()).Line
			exported := d.Name.IsExported()

			functions = append(functions, models.Function{
				Name:      d.Name.Name,
				StartLine: start,
				EndLine:   end,
				Exported:  exported,
			})

			symbol := models.Symbol{
				Name:      d.Name.Name,
				FilePath:  path,
				StartLine: start,
				EndLine:   end,
			}

			if d.Recv != nil && len(d.Recv.List) > 0 {
				// It's a method
				symbol.Kind = models.MethodSymbol
				receiverType := d.Recv.List[0].Type
				// Handle pointer receivers
				if star, ok := receiverType.(*ast.StarExpr); ok {
					receiverType = star.X
				}
				if ident, ok := receiverType.(*ast.Ident); ok {
					symbol.Receiver = ident.Name
				}
			} else {
				// It's a function
				symbol.Kind = models.FunctionSymbol
			}
			symbols = append(symbols, symbol)

		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					ts := spec.(*ast.TypeSpec)
					start := fset.Position(ts.Pos()).Line
					end := fset.Position(ts.End()).Line

					symbol := models.Symbol{
						Name:      ts.Name.Name,
						FilePath:  path,
						StartLine: start,
						EndLine:   end,
					}

					switch ts.Type.(type) {
					case *ast.StructType:
						symbol.Kind = models.StructSymbol
					case *ast.InterfaceType:
						symbol.Kind = models.InterfaceSymbol
					default:
						continue
					}
					symbols = append(symbols, symbol)
				}
			}
		}
	}

	// Second pass: extract references for each symbol
	for i := range symbols {
		symbols[i].References = p.extractReferences(f, &symbols[i])
	}

	return &models.File{
		Path:      path,
		Package:   "package " + pkgName,
		Imports:   imports,
		Functions: functions,
		Symbols:   symbols,
	}, nil
}

// extractReferences finds types referenced by a symbol (fields, params, returns).
func (p *GoParser) extractReferences(f *ast.File, sym *models.Symbol) []string {
	var refs []string
	seen := make(map[string]struct{})

	addRef := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; !ok {
			refs = append(refs, name)
			seen[name] = struct{}{}
		}
	}

	// Find the AST node for this symbol
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == sym.Name {
				// Check receiver for methods
				if sym.Kind == models.MethodSymbol && d.Recv != nil {
					// We already know the receiver, but let's check if it references anything else
				}
				if sym.Kind == models.FunctionSymbol || sym.Kind == models.MethodSymbol {
					// Parameters
					if d.Type.Params != nil {
						for _, field := range d.Type.Params.List {
							p.collectTypeRefs(field.Type, addRef)
						}
					}
					// Return types
					if d.Type.Results != nil {
						for _, field := range d.Type.Results.List {
							p.collectTypeRefs(field.Type, addRef)
						}
					}
				}
			}
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					ts := spec.(*ast.TypeSpec)
					if ts.Name.Name == sym.Name {
						switch t := ts.Type.(type) {
						case *ast.StructType:
							for _, field := range t.Fields.List {
								p.collectTypeRefs(field.Type, addRef)
							}
						case *ast.InterfaceType:
							for _, method := range t.Methods.List {
								p.collectTypeRefs(method.Type, addRef)
							}
						}
					}
				}
			}
		}
	}
	return refs
}

func (p *GoParser) collectTypeRefs(expr ast.Expr, add func(string)) {
	switch e := expr.(type) {
	case *ast.Ident:
		add(e.Name)
	case *ast.SelectorExpr:
		// External package reference like "models.File"
		// For now, we only care about the type name in our own analysis if it's internal
		// but we can store the whole thing "pkg.Type"
		if x, ok := e.X.(*ast.Ident); ok {
			add(x.Name + "." + e.Sel.Name)
		}
	case *ast.StarExpr:
		p.collectTypeRefs(e.X, add)
	case *ast.ArrayType:
		p.collectTypeRefs(e.Elt, add)
	case *ast.MapType:
		p.collectTypeRefs(e.Key, add)
		p.collectTypeRefs(e.Value, add)
	case *ast.ChanType:
		p.collectTypeRefs(e.Value, add)
	case *ast.FuncType:
		if e.Params != nil {
			for _, f := range e.Params.List {
				p.collectTypeRefs(f.Type, add)
			}
		}
		if e.Results != nil {
			for _, f := range e.Results.List {
				p.collectTypeRefs(f.Type, add)
			}
		}
	}
}
