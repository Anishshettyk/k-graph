package query

import (
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/anishetty/kgraph/internal/graph"
)

// chainGraph builds Service→Pod plus Deployment→ReplicaSet→Pod so both forward
// and reverse traversals have something to walk.
//
//	deployment ──owns──▶ replicaset ──owns──▶ pod ◀──selects── service
func chainGraph() (*graph.Graph, map[string]types.UID) {
	g := graph.New()
	ids := map[string]types.UID{
		"deploy": "deploy-1",
		"rs":     "rs-1",
		"pod":    "pod-1",
		"svc":    "svc-1",
	}
	g.AddNode(graph.Node{UID: ids["deploy"], Kind: "Deployment", Name: "frontend"})
	g.AddNode(graph.Node{UID: ids["rs"], Kind: "ReplicaSet", Name: "frontend-abc"})
	g.AddNode(graph.Node{UID: ids["pod"], Kind: "Pod", Name: "frontend-abc-xyz"})
	g.AddNode(graph.Node{UID: ids["svc"], Kind: "Service", Name: "frontend"})

	g.AddEdge(ids["deploy"], ids["rs"], graph.RelOwns)
	g.AddEdge(ids["rs"], ids["pod"], graph.RelOwns)
	g.AddEdge(ids["svc"], ids["pod"], graph.RelSelects)
	return g, ids
}

func uidsAtDepth(results []Result) map[types.UID]int {
	m := make(map[types.UID]int, len(results))
	for _, r := range results {
		m[r.Node.UID] = r.Depth
	}
	return m
}

func TestDependencies(t *testing.T) {
	g, ids := chainGraph()

	got := uidsAtDepth(Dependencies(g, ids["deploy"]))

	if len(got) != 2 {
		t.Fatalf("expected 2 dependencies, got %d: %v", len(got), got)
	}
	if got[ids["rs"]] != 1 {
		t.Errorf("replicaset depth = %d, want 1", got[ids["rs"]])
	}
	if got[ids["pod"]] != 2 {
		t.Errorf("pod depth = %d, want 2", got[ids["pod"]])
	}
	if _, ok := got[ids["deploy"]]; ok {
		t.Error("start node should not appear in its own dependencies")
	}
}

func TestImpact(t *testing.T) {
	g, ids := chainGraph()

	got := uidsAtDepth(Impact(g, ids["pod"]))

	// Pod is pointed at by ReplicaSet (owns) and Service (selects); ReplicaSet
	// is pointed at by Deployment.
	if len(got) != 3 {
		t.Fatalf("expected 3 impacted nodes, got %d: %v", len(got), got)
	}
	if got[ids["rs"]] != 1 || got[ids["svc"]] != 1 {
		t.Errorf("direct dependents should be depth 1, got rs=%d svc=%d", got[ids["rs"]], got[ids["svc"]])
	}
	if got[ids["deploy"]] != 2 {
		t.Errorf("deployment depth = %d, want 2", got[ids["deploy"]])
	}
}

func TestTraversalHandlesNoNeighbors(t *testing.T) {
	g := graph.New()
	lone := types.UID("lone")
	g.AddNode(graph.Node{UID: lone, Kind: "Pod", Name: "lonely"})

	if got := Dependencies(g, lone); len(got) != 0 {
		t.Errorf("expected no dependencies, got %v", got)
	}
	if got := Impact(g, lone); len(got) != 0 {
		t.Errorf("expected no impact, got %v", got)
	}
}
