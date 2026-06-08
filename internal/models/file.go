package models

type FileRole string

const (
	RoleEntrypoint     FileRole = "entrypoint"
	RoleCoreLogic      FileRole = "core_logic"
	RoleInterface      FileRole = "interface"
	RoleInfrastructure FileRole = "infrastructure"
	RoleTest           FileRole = "test"
	RoleGenerated      FileRole = "generated"
	RoleExample        FileRole = "example"
	RoleConfig         FileRole = "config"
)

// File represents a Go source file and its extracted metadata.
type File struct {
	// Path is the absolute or relative path to the file.
	Path string
	// Package is the package declaration (e.g., "package main").
	Package string
	// Role is the classified purpose of the file.
	Role FileRole
	// Imports is a list of imported package paths.
	Imports []string
	// Functions is a list of functions defined in the file.
	Functions []Function
	// Blocks is a list of code blocks identified in the file.
	Blocks []CodeBlock
	// Score is a calculated metric for the file's importance or complexity.
	Score int64
}
