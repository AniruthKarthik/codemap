package models

// LearningUnit represents a cohesive set of symbols to be learned together.
type LearningUnit struct {
	Order int
	ID    string
	Name  string

	// SymbolIDs is a list of symbol identifiers in this unit.
	SymbolIDs []string

	// PrerequisiteIDs is a list of concepts that should be learned before this one.
	PrerequisiteIDs []string

	Reason string

	// EstimatedTimeMinutes is the approximate time to understand this unit.
	EstimatedTimeMinutes int

	// KnowledgeGainPercentage is the estimated portion of the system understood after this unit.
	KnowledgeGainPercentage float64

	// Coverage is the system understanding state after this unit.
	Coverage Coverage

	Importance float64
}

// LearningStep is a legacy model for file-based paths (Phase 13).
type LearningStep struct {
	Order  int
	File   string
	Reason string
	Score  float64
}
