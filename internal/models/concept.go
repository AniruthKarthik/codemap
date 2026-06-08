package models

type ConceptRole string

const (
	RoleFoundational ConceptRole = "foundational"
	RoleCore         ConceptRole = "core"
	RoleSupporting   ConceptRole = "supporting"
	RolePeripheral   ConceptRole = "peripheral"
)

// ConceptSummary provides a human-readable explanation of why a concept matters.
type ConceptSummary struct {
	Purpose  string
	WhyLearn string
	Unlocks  []string
}

// Concept represents a logical cluster of symbols that form a high-level feature or flow.
type Concept struct {
	ID   string
	Name string

	Role ConceptRole

	Summary ConceptSummary

	// SymbolIDs is a list of symbol identifiers belonging to this concept.
	SymbolIDs []string

	// PrerequisiteIDs is a list of concepts that should be learned before this one.
	PrerequisiteIDs []string

	// Cohesion is a measure of how tightly symbols within the concept are connected (0.0 to 1.0).
	Cohesion float64

	// Importance is the calculated architectural weight of the concept.
	Importance float64
}

// ConceptEdge represents a dependency between two concepts.
type ConceptEdge struct {
	From string // Concept ID
	To   string // Concept ID
}
