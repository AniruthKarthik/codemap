package models

type Node struct {
	FilePath string
}

type Edge struct {
	From Node
	To   Node
}

type Graph struct {
	Nodes []Node
	Edges []Edge
}
