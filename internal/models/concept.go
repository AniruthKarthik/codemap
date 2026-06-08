package models

// Concept represents a logical cluster of symbols that form a high-level feature or flow.
type Concept struct {
	Name string

	// SymbolIDs is a list of symbol identifiers belonging to this concept.
	SymbolIDs []string

	// Score is the calculated importance of the concept.
	Score float64
}
