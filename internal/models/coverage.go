package models

// Coverage represents the extent of the system understood after completing certain learning units.
type Coverage struct {
	SymbolsCovered  int
	TotalSymbols    int
	ConceptsCovered int
	TotalConcepts   int

	SymbolPercentage  float64
	ConceptPercentage float64
}
