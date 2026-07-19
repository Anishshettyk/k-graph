// Package query implements deterministic graph traversals over the KGraph
// knowledge graph. Every CLI command is expressed as one of these traversals.
package query

import (
	"k8s.io/apimachinery/pkg/types"

	"github.com/anishetty/kgraph/internal/graph"
)

// Result is a node reached during a traversal, along with its BFS depth from
// the starting node (the start itself is depth 0 and is not included in
// results).
type Result struct {
	Node  graph.Node
	Depth int
}

// Dependencies returns everything the start node (transitively) depends on or
// owns, by following forward edges. Example: Deployment → ReplicaSet → Pod.
func Dependencies(g *graph.Graph, start types.UID) []Result {
	return bfs(g, start, g.Out)
}

// Impact returns everything that (transitively) depends on the start node, by
// following reverse edges. Answers "what breaks if I change this?".
func Impact(g *graph.Graph, start types.UID) []Result {
	return bfs(g, start, g.In)
}

// bfs performs a breadth-first traversal from start using the supplied neighbor
// function (g.Out for forward, g.In for reverse). Traversal order is
// deterministic because edges are stored in insertion order.
func bfs(g *graph.Graph, start types.UID, neighbors func(types.UID) []types.UID) []Result {
	type item struct {
		uid   types.UID
		depth int
	}

	visited := map[types.UID]bool{start: true}
	queue := []item{{uid: start, depth: 0}}
	var results []Result

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, next := range neighbors(cur.uid) {
			if visited[next] {
				continue
			}
			visited[next] = true

			node, ok := g.Node(next)
			if !ok {
				continue
			}
			depth := cur.depth + 1
			results = append(results, Result{Node: node, Depth: depth})
			queue = append(queue, item{uid: next, depth: depth})
		}
	}

	return results
}
