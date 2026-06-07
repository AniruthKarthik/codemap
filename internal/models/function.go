package models

// Function represents a function declaration within a Go source file.
type Function struct {
	// Name is the name of the function.
	Name string
	// StartLine is the 1-based line number where the function starts.
	StartLine int
	// EndLine is the 1-based line number where the function ends.
	EndLine int
	// Exported indicates if the function is exported (starts with an uppercase letter).
	Exported bool
}
