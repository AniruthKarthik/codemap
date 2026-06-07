package models

// File represents a Go source file and its extracted metadata.
type File struct {
	// Path is the absolute or relative path to the file.
	Path string
	// Package is the package declaration (e.g., "package main").
	Package string
	// Imports is a list of imported package paths.
	Imports []string
	// Functions is a list of functions defined in the file.
	Functions []Function
	// Score is a calculated metric for the file's importance or complexity.
	Score int64
}
