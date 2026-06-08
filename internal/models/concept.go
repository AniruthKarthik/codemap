package models

type ConceptRole string

const (
	RoleFoundational ConceptRole = "foundational"
	RoleCore         ConceptRole = "core"
	RoleSupporting   ConceptRole = "supporting"
	RolePeripheral   ConceptRole = "peripheral"
)

type ConceptType string

const (
	CoreAbstraction ConceptType = "core_abstraction"
	EntryAPI        ConceptType = "entry_api"
	SupportingType  ConceptType = "supporting_type"
	UtilityType     ConceptType = "utility_type"
)

// Rankings holds the different ways the system can order concepts.
type Rankings struct {
	ArchitectureRanking []string // Concept IDs ordered by structural importance
	LearningRanking     []string // Concept IDs ordered by learning sequence
}

// ConceptSummary provides a human-readable explanation of why a concept matters.
type ConceptSummary struct {
	Purpose           string
	WhyLearn          string
	LearningObjective string
	Unlocks           []string
}

// ImportanceFactors details why a concept was assigned its architectural weight.
type ImportanceFactors struct {
	Centrality        float64
	Reachability      float64
	DependencyUnlocks float64
	PublicAPIWeight   float64
	ConceptSize       float64
}

// Concept represents a logical cluster of symbols that form a high-level feature or flow.
type Concept struct {
	ID   string
	Name string

	Role ConceptRole
	Type ConceptType

	Summary ConceptSummary

	Factors ImportanceFactors

	// SymbolIDs is a list of symbol identifiers belonging to this concept.
	SymbolIDs []string

	// PrerequisiteIDs is a list of concepts that should be learned before this one.
	PrerequisiteIDs []string

	// Cohesion is a measure of how tightly symbols within the concept are connected (0.0 to 1.0).
	Cohesion float64

	// Importance is the calculated architectural weight of the concept.
	Importance float64

	// LearningPriority is the rank in which a developer should learn this concept.
	LearningPriority float64
}

// ConceptEdge represents a dependency between two concepts.
type ConceptEdge struct {
	From string // Concept ID
	To   string // Concept ID
}
