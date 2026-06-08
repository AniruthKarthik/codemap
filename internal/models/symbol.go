package models

type SymbolKind string

const (
	StructSymbol    SymbolKind = "struct"
	InterfaceSymbol SymbolKind = "interface"
	FunctionSymbol  SymbolKind = "function"
	MethodSymbol    SymbolKind = "method"
)

// Symbol represents an architectural element like a struct, interface, or function.
type Symbol struct {
	// ID is a unique identifier, e.g., "github.com/user/repo.Struct.Method"
	ID string

	Name string

	Kind SymbolKind

	// Package is the fully qualified package path.
	Package string

	// Receiver is the name of the struct/interface for methods.
	Receiver string

	// References is a list of type names or symbol names referenced by this symbol.
	References []string

	FilePath string

	StartLine int
	EndLine   int

	// Score is the calculated importance of the symbol.
	Score float64
}

// SymbolEdge represents a reference from one symbol to another.
type SymbolEdge struct {
	From string // Symbol ID
	To   string // Symbol ID
}
