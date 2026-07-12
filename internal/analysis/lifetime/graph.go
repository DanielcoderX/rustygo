package lifetime

import (
	"fmt"

	"golang.org/x/tools/go/ssa"
)

// GraphNode represents a node in the lifetime flow graph.
type GraphNode struct {
	ID    string
	Val   ssa.Value
	Instr ssa.Instruction
	Name  string
}

// GraphEdge represents a directed edge showing value flow.
type GraphEdge struct {
	From *GraphNode
	To   *GraphNode
	Type string
}

// LifetimeGraph represents the explicit flow graph of allocations.
type LifetimeGraph struct {
	Nodes    map[string]*GraphNode
	Edges    []*GraphEdge
	Adjacency map[string][]*GraphEdge
}

// NewLifetimeGraph initializes a new lifetime graph.
func NewLifetimeGraph() *LifetimeGraph {
	return &LifetimeGraph{
		Nodes:    make(map[string]*GraphNode),
		Adjacency: make(map[string][]*GraphEdge),
	}
}

// GetOrCreateNode returns the unique node for a value/instruction.
func (g *LifetimeGraph) GetOrCreateNode(val ssa.Value, instr ssa.Instruction, name string) *GraphNode {
	id := ""
	if val != nil {
		id = fmt.Sprintf("val_%p", val)
	} else if instr != nil {
		id = fmt.Sprintf("instr_%p", instr)
	} else {
		id = fmt.Sprintf("name_%s", name)
	}

	if node, ok := g.Nodes[id]; ok {
		return node
	}

	node := &GraphNode{
		ID:    id,
		Val:   val,
		Instr: instr,
		Name:  name,
	}
	g.Nodes[id] = node
	return node
}

// AddEdge adds a directed edge to the graph.
func (g *LifetimeGraph) AddEdge(from, to *GraphNode, edgeType string) {
	edge := &GraphEdge{
		From: from,
		To:   to,
		Type: edgeType,
	}
	g.Edges = append(g.Edges, edge)
	g.Adjacency[from.ID] = append(g.Adjacency[from.ID], edge)
}
