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
			
			if d.Body != nil {
				symbol.Blocks = p.classifyBlocks(fset, d.Body)
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
		Path:       path,
		Package:    "package " + pkgName,
		Imports:    imports,
		Functions:  functions,
		Symbols:    symbols,
		TotalLines: fset.File(f.Package).LineCount(),
	}, nil
}

// extractReferences finds types referenced by a symbol (fields, params, returns, constructs, embeds).
func (p *GoParser) extractReferences(f *ast.File, sym *models.Symbol) []models.Reference {
	var refs []models.Reference
	seen := make(map[string]struct{})

	addRef := func(name string, refType models.ReferenceType) {
		if name == "" {
			return
		}
		key := name + string(refType)
		if _, ok := seen[key]; !ok {
			refs = append(refs, models.Reference{Name: name, Type: refType})
			seen[key] = struct{}{}
		}
	}

	// Find the AST node for this symbol
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == sym.Name {
				if sym.Kind == models.FunctionSymbol || sym.Kind == models.MethodSymbol {
					isConstructor := strings.HasPrefix(sym.Name, "New")

					// Receiver (for methods)
					if d.Recv != nil && len(d.Recv.List) > 0 {
						p.collectTypeRefs(d.Recv.List[0].Type, models.RefField, addRef)
					}

					// Parameters
					if d.Type.Params != nil {
						for _, field := range d.Type.Params.List {
							p.collectTypeRefs(field.Type, models.RefParameter, addRef)
						}
					}
					// Return types
					if d.Type.Results != nil {
						for _, field := range d.Type.Results.List {
							refType := models.RefReturn
							if isConstructor {
								targetName := strings.TrimPrefix(sym.Name, "New")
								if p.isTypeMatch(field.Type, targetName) {
									refType = models.RefConstruct
								}
							}
							p.collectTypeRefs(field.Type, refType, addRef)
						}
					}

					// Function Calls inside the body
					if d.Body != nil {
						ast.Inspect(d.Body, func(n ast.Node) bool {
							if call, ok := n.(*ast.CallExpr); ok {
								if ident, ok := call.Fun.(*ast.Ident); ok {
									addRef(ident.Name, models.RefCall)
								} else if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
									addRef(sel.Sel.Name, models.RefCall)
								}
							}
							return true
						})
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
								refType := models.RefField
								if field.Names == nil {
									refType = models.RefEmbed
								}
								p.collectTypeRefs(field.Type, refType, addRef)
							}
						case *ast.InterfaceType:
							for _, method := range t.Methods.List {
								if len(method.Names) > 0 {
									addRef(method.Names[0].Name, models.RefCall)
								}
								p.collectTypeRefs(method.Type, models.RefParameter, addRef)
							}
						}
					}
				}
			}
		}
	}
	return refs
}

func (p *GoParser) isTypeMatch(expr ast.Expr, targetName string) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == targetName
	case *ast.StarExpr:
		return p.isTypeMatch(e.X, targetName)
	}
	return false
}

func (p *GoParser) collectTypeRefs(expr ast.Expr, refType models.ReferenceType, add func(string, models.ReferenceType)) {
	switch e := expr.(type) {
	case *ast.Ident:
		add(e.Name, refType)
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok {
			add(x.Name+"."+e.Sel.Name, refType)
		}
	case *ast.StarExpr:
		p.collectTypeRefs(e.X, refType, add)
	case *ast.ArrayType:
		p.collectTypeRefs(e.Elt, refType, add)
	case *ast.MapType:
		p.collectTypeRefs(e.Key, refType, add)
		p.collectTypeRefs(e.Value, refType, add)
	case *ast.ChanType:
		p.collectTypeRefs(e.Value, refType, add)
	case *ast.FuncType:
		if e.Params != nil {
			for _, f := range e.Params.List {
				p.collectTypeRefs(f.Type, refType, add)
			}
		}
		if e.Results != nil {
			for _, f := range e.Results.List {
				p.collectTypeRefs(f.Type, refType, add)
			}
		}
	}
}

func (p *GoParser) classifyBlocks(fset *token.FileSet, body *ast.BlockStmt) []models.CodeBlock {
	var blocks []models.CodeBlock
	if body == nil {
		return blocks
	}

	for _, stmt := range body.List {
		start := fset.Position(stmt.Pos()).Line
		end := fset.Position(stmt.End()).Line

		category := models.CategoryExecutionFlow
		visibility := models.VisibilityUseful
		reason := "Execution Flow"

		isNoise := false
		isCritical := false

		ast.Inspect(stmt, func(n ast.Node) bool {
			if n == nil {
				return false
			}
			switch x := n.(type) {
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					if ident, ok := sel.X.(*ast.Ident); ok {
						pkg := ident.Name
						method := sel.Sel.Name
						
						if pkg == "logger" || pkg == "log" || pkg == "zap" || pkg == "logrus" || method == "Info" || method == "Debug" || method == "Error" || method == "Warn" || method == "Printf" || method == "Println" {
							category = models.CategoryLogging
							isNoise = true
							reason = "Logging"
						} else if pkg == "metrics" || pkg == "stats" || pkg == "prometheus" || method == "Record" || method == "Inc" || method == "Observe" {
							category = models.CategoryMetrics
							isNoise = true
							reason = "Metrics"
						} else if pkg == "span" || pkg == "trace" || pkg == "tracer" || method == "AddEvent" {
							category = models.CategoryNoise
							isNoise = true
							reason = "Tracing"
						} else if method == "Execute" || method == "Generate" || method == "Save" || method == "Run" || method == "Start" || method == "Stop" || method == "Update" {
							category = models.CategoryIntegration
							isCritical = true
							reason = "Cross-System Call"
						} else if strings.HasPrefix(method, "New") {
							category = models.CategoryIntegration
							isCritical = true
							reason = "Dependency Wiring"
						}
					}
				} else if ident, ok := x.Fun.(*ast.Ident); ok {
					if strings.HasPrefix(ident.Name, "New") || ident.Name == "make" {
						category = models.CategoryIntegration
						isCritical = true
						reason = "Dependency Wiring"
					}
				}
			case *ast.ReturnStmt:
				if len(x.Results) > 0 {
					if call, ok := x.Results[0].(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							if sel.Sel.Name == "Errorf" {
								category = models.CategoryErrorHandling
								isNoise = true
								reason = "Error Wrapping"
							}
						}
					}
				}
			case *ast.IfStmt:
				if p.isErrCheck(x) {
					if p.isSimpleErrorReturn(x) {
						category = models.CategoryErrorHandling
						isNoise = true
						reason = "Error Handling"
					}
				} else if p.isValidation(x) {
					category = models.CategoryValidation
					isNoise = true
					reason = "Simple Validation"
				}
			}
			return true
		})

		if isNoise {
			visibility = models.VisibilityNoise
		} else if isCritical {
			visibility = models.VisibilityCritical
		}

		blocks = append(blocks, models.CodeBlock{
			StartLine:  start,
			EndLine:    end,
			Category:   category,
			Visibility: visibility,
			Reason:     reason,
		})
	}
	return blocks
}

func (p *GoParser) isErrCheck(ifStmt *ast.IfStmt) bool {
	if bin, ok := ifStmt.Cond.(*ast.BinaryExpr); ok {
		if bin.Op == token.NEQ {
			if id, ok := bin.X.(*ast.Ident); ok && id.Name == "err" {
				return true
			}
			if id, ok := bin.Y.(*ast.Ident); ok && id.Name == "nil" {
				return true
			}
		}
	}
	if ifStmt.Init != nil {
		if assign, ok := ifStmt.Init.(*ast.AssignStmt); ok {
			for _, lhs := range assign.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == "err" {
					return true
				}
			}
		}
	}
	return false
}

func (p *GoParser) isValidation(ifStmt *ast.IfStmt) bool {
	if bin, ok := ifStmt.Cond.(*ast.BinaryExpr); ok {
		if bin.Op == token.EQL {
			if lit, ok := bin.Y.(*ast.BasicLit); ok && lit.Value == `""` {
				return true
			}
			if id, ok := bin.Y.(*ast.Ident); ok && id.Name == "nil" {
				return true
			}
		}
	}
	return false
}

func (p *GoParser) isSimpleErrorReturn(ifStmt *ast.IfStmt) bool {
	if ifStmt.Body == nil {
		return false
	}
	for _, stmt := range ifStmt.Body.List {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.ExprStmt:
			if call, ok := s.X.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok {
						if id.Name == "log" || id.Name == "logger" {
							continue
						}
					}
				}
			}
			return false
		default:
			return false
		}
	}
	return true
}
