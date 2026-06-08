package models

// LineRange represents a contiguous block of lines in a file.
type LineRange struct {
	Start int
	End   int
}

// Highlight represents a specific range of lines to be emphasized.
type Highlight struct {
	Start  int
	End    int
	Reason string
}

// FileSlice represents the parts of a single file that are relevant to a concept.
type FileSlice struct {
	FilePath   string
	Ranges     []LineRange
	Highlights []Highlight
}

// CodeSlice represents the minimal set of code needed to understand a concept.
type CodeSlice struct {
	ConceptID string

	// Files is a list of file segments relevant to this concept.
	Files []FileSlice

	// HiddenLinesCount is the total number of lines omitted from the files involved.
	HiddenLinesCount int
}
