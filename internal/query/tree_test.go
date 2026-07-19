package query

import (
	"testing"

	"github.com/anishetty/kgraph/internal/graph"
)

func TestDependencyTree(t *testing.T) {
	g, ids := chainGraph()
	root, _ := g.Node(ids["deploy"])

	tree := DependencyTree(g, root)

	if tree.Node.UID != ids["deploy"] {
		t.Fatalf("root = %v, want deploy", tree.Node.UID)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("deploy should have 1 child, got %d", len(tree.Children))
	}
	rs := tree.Children[0]
	if rs.Node.UID != ids["rs"] || rs.Rel != graph.RelOwns {
		t.Errorf("child = %v rel=%v, want rs/owns", rs.Node.UID, rs.Rel)
	}
	if len(rs.Children) != 1 || rs.Children[0].Node.UID != ids["pod"] {
		t.Errorf("replicaset should own the pod, got %+v", rs.Children)
	}
}

func TestImpactTree(t *testing.T) {
	g, ids := chainGraph()
	root, _ := g.Node(ids["pod"])

	tree := ImpactTree(g, root)

	// Pod is pointed at by ReplicaSet (owns) and Service (selects).
	if len(tree.Children) != 2 {
		t.Fatalf("pod should have 2 dependents, got %d", len(tree.Children))
	}
	kinds := map[string]bool{}
	for _, c := range tree.Children {
		kinds[c.Node.Kind] = true
	}
	if !kinds["ReplicaSet"] || !kinds["Service"] {
		t.Errorf("expected ReplicaSet and Service dependents, got %v", kinds)
	}
}

func TestTreeCycleGuard(t *testing.T) {
	g := graph.New()
	a := graph.Node{UID: "a", Kind: "Pod", Name: "a"}
	b := graph.Node{UID: "b", Kind: "Pod", Name: "b"}
	g.AddNode(a)
	g.AddNode(b)
	// Create a cycle a -> b -> a.
	g.AddEdge("a", "b", graph.RelReferences)
	g.AddEdge("b", "a", graph.RelReferences)

	tree := DependencyTree(g, a)
	// a -> b -> a(cycle marker), then stop.
	if len(tree.Children) != 1 || tree.Children[0].Node.UID != "b" {
		t.Fatalf("expected single child b, got %+v", tree.Children)
	}
	bNode := tree.Children[0]
	if len(bNode.Children) != 1 || !bNode.Children[0].Cycle {
		t.Errorf("expected cycle-marked child back to a, got %+v", bNode.Children)
	}
}
