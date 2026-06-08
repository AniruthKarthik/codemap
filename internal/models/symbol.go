package models

type SymbolKind string

const (
	StructSymbol    SymbolKind = "struct"
	InterfaceSymbol SymbolKind = "interface"
	FunctionSymbol  SymbolKind = "function"
	MethodSymbol    SymbolKind = "method"
)

type ReferenceType string

const (
	RefField     ReferenceType = "field"
	RefParameter ReferenceType = "parameter"
	RefReturn    ReferenceType = "return"
	RefEmbed     ReferenceType = "embed"
	RefImplement ReferenceType = "implements"
	RefConstruct ReferenceType = "constructs"
	RefCall      ReferenceType = "calls"
)

// Reference represents a typed reference to another symbol.
type Reference struct {
	Name string
	Type ReferenceType
}

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

	// References is a list of typed references to other symbols.
	References []Reference

	FilePath string

	StartLine int
	EndLine   int

	// Score is the calculated importance of the symbol.
	Score float64

	// Factors details the components of the symbol's score.
	Factors ImportanceFactors
}

// SymbolEdge represents a reference from one symbol to another.
type SymbolEdge struct {
	From string        // Symbol ID
	To   string        // Symbol ID
	Type ReferenceType // Type of reference
}
