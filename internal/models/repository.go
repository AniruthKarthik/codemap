package models

// Repository represents an indexed collection of Go source files.
type Repository struct {
	// Files is a slice of File models belonging to the repository.
	Files []*File
	// SymbolEdges represents the reference graph between symbols.
	SymbolEdges []SymbolEdge
}
