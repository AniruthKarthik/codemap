package models

type CodeCategory string

const (
	CategoryEntrypoint    CodeCategory = "Entrypoint"
	CategoryCoreDomain    CodeCategory = "CoreDomain"
	CategoryExecutionFlow CodeCategory = "ExecutionFlow"
	CategoryPublicAPI     CodeCategory = "PublicAPI"
	CategoryIntegration   CodeCategory = "Integration"
	CategoryInfrastructure CodeCategory = "Infrastructure"
	CategoryUtility       CodeCategory = "Utility"
	CategoryErrorHandling CodeCategory = "ErrorHandling"
	CategoryLogging       CodeCategory = "Logging"
	CategoryMetrics       CodeCategory = "Metrics"
	CategoryValidation    CodeCategory = "Validation"
	CategoryTest          CodeCategory = "Test"
	CategoryNoise         CodeCategory = "Noise"
)

type VisibilityLevel string

const (
	VisibilityCritical VisibilityLevel = "Critical"
	VisibilityUseful   VisibilityLevel = "Useful"
	VisibilityAdvanced VisibilityLevel = "Advanced"
	VisibilityNoise    VisibilityLevel = "Noise"
)

// CodeBlock represents a discrete segment of code within a file.
type CodeBlock struct {
	Name       string
	StartLine  int
	EndLine    int
	Score      float64
	Category   CodeCategory
	Visibility VisibilityLevel
	Reason     string
}
