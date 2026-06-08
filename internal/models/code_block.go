package models

// CodeBlock represents a discrete segment of code within a file.
type CodeBlock struct {
	Name      string
	StartLine int
	EndLine   int
	Score     float64
}
