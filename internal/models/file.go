package models

type File struct {
	Path      string
	Package   string
	Imports   []string
	Functions []Function
	Score     int64
}
