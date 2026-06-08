package models

// Repository represents an indexed collection of Go source files.
type Repository struct {
	// Files is a slice of File models belonging to the repository.
	Files []*File
	// SymbolEdges represents the reference graph between symbols.
	SymbolEdges []SymbolEdge
	// Concepts represents logical clusters of symbols.
	Concepts []Concept
	// ConceptEdges represents dependencies between concepts.
	ConceptEdges []ConceptEdge
	// Rankings contains the prioritized lists of concepts.
	Rankings Rankings
}
