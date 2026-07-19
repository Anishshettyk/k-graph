package query

import (
	"k8s.io/apimachinery/pkg/types"

	"github.com/anishetty/kgraph/internal/graph"
)

// TreeNode is a node in a rendered dependency/impact tree. Rel is the
// relationship of the edge that led from the parent to this node (empty for the
// root). Cycle is true when the node was already visited elsewhere in the tree,
// in which case Children is not expanded again.
type TreeNode struct {
	Node     graph.Node
	Rel      graph.Relationship
	Children []*TreeNode
	Cycle    bool
}

// DependencyTree builds a tree rooted at start by following forward edges
// (what start depends on / owns).
func DependencyTree(g *graph.Graph, start graph.Node) *TreeNode {
	return buildTree(g, start, forward, 0)
}

// ImpactTree builds a tree rooted at start by following reverse edges
// (what depends on start).
func ImpactTree(g *graph.Graph, start graph.Node) *TreeNode {
	return buildTree(g, start, reverse, 0)
}

// DependencyTreeDepth is DependencyTree limited to maxDepth levels (0 = unlimited).
func DependencyTreeDepth(g *graph.Graph, start graph.Node, maxDepth int) *TreeNode {
	return buildTree(g, start, forward, maxDepth)
}

// ImpactTreeDepth is ImpactTree limited to maxDepth levels (0 = unlimited).
func ImpactTreeDepth(g *graph.Graph, start graph.Node, maxDepth int) *TreeNode {
	return buildTree(g, start, reverse, maxDepth)
}

// direction abstracts forward vs. reverse edge walking while preserving the
// relationship label and the neighbor node on the far end of each edge.
type direction func(g *graph.Graph, uid types.UID) []step

type step struct {
	rel graph.Relationship
	to  types.UID
}

func forward(g *graph.Graph, uid types.UID) []step {
	edges := g.OutEdges(uid)
	steps := make([]step, 0, len(edges))
	for _, e := range edges {
		steps = append(steps, step{rel: e.Rel, to: e.To})
	}
	return steps
}

func reverse(g *graph.Graph, uid types.UID) []step {
	edges := g.InEdges(uid)
	steps := make([]step, 0, len(edges))
	for _, e := range edges {
		steps = append(steps, step{rel: e.Rel, to: e.From})
	}
	return steps
}

func buildTree(g *graph.Graph, start graph.Node, dir direction, maxDepth int) *TreeNode {
	root := &TreeNode{Node: start}
	visited := map[types.UID]bool{start.UID: true}
	expand(g, root, dir, visited, 1, maxDepth)
	return root
}

// expand recursively attaches children to parent, guarding against cycles so a
// diamond or loop in the graph does not cause infinite recursion. depth is the
// level of parent's children; when maxDepth > 0, recursion stops past it.
func expand(g *graph.Graph, parent *TreeNode, dir direction, visited map[types.UID]bool, depth, maxDepth int) {
	if maxDepth > 0 && depth > maxDepth {
		return
	}
	for _, s := range dir(g, parent.Node.UID) {
		node, ok := g.Node(s.to)
		if !ok {
			continue
		}
		child := &TreeNode{Node: node, Rel: s.rel}
		if visited[s.to] {
			child.Cycle = true
			parent.Children = append(parent.Children, child)
			continue
		}
		visited[s.to] = true
		parent.Children = append(parent.Children, child)
		expand(g, child, dir, visited, depth+1, maxDepth)
	}
}
