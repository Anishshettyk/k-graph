package graph

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func edgeSet(edges []Edge) map[Edge]bool {
	set := make(map[Edge]bool, len(edges))
	for _, e := range edges {
		set[e] = true
	}
	return set
}

func TestBuildEdges(t *testing.T) {
	const (
		deployUID = types.UID("deploy-1")
		rsUID     = types.UID("rs-1")
		podUID    = types.UID("pod-1")
		svcUID    = types.UID("svc-1")
		orphanUID = types.UID("rs-orphan")
	)

	podLabels := map[string]string{"app": "frontend"}

	res := Resources{
		Deployments: []appsv1.Deployment{
			{ObjectMeta: metav1.ObjectMeta{UID: deployUID, Name: "frontend", Namespace: "default"}},
		},
		ReplicaSets: []appsv1.ReplicaSet{
			{ObjectMeta: metav1.ObjectMeta{
				UID: rsUID, Name: "frontend-abc", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{UID: deployUID, Kind: "Deployment"}},
			}},
			// Orphan RS: owner not present in the snapshot, so no owner edge.
			{ObjectMeta: metav1.ObjectMeta{
				UID: orphanUID, Name: "orphan", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{UID: "missing", Kind: "Deployment"}},
			}},
		},
		Pods: []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{
				UID: podUID, Name: "frontend-abc-xyz", Namespace: "default", Labels: podLabels,
				OwnerReferences: []metav1.OwnerReference{{UID: rsUID, Kind: "ReplicaSet"}},
			}},
		},
		Services: []corev1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{UID: svcUID, Name: "frontend", Namespace: "default"},
				Spec:       corev1.ServiceSpec{Selector: podLabels},
			},
		},
	}

	g := Build(res)

	if got := len(g.Nodes()); got != 5 {
		t.Fatalf("expected 5 nodes, got %d", got)
	}

	edges := edgeSet(g.Edges())

	tests := []struct {
		name string
		edge Edge
		want bool
	}{
		{"deployment owns replicaset", Edge{From: deployUID, To: rsUID, Rel: RelOwns}, true},
		{"replicaset owns pod", Edge{From: rsUID, To: podUID, Rel: RelOwns}, true},
		{"service selects pod", Edge{From: svcUID, To: podUID, Rel: RelSelects}, true},
		{"no edge for orphan owner", Edge{From: "missing", To: orphanUID, Rel: RelOwns}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if edges[tt.edge] != tt.want {
				t.Errorf("edge %+v present=%v, want %v", tt.edge, edges[tt.edge], tt.want)
			}
		})
	}
}

func TestOutAndIn(t *testing.T) {
	g := New()
	a := types.UID("a")
	b := types.UID("b")
	g.AddNode(Node{UID: a, Kind: "Service", Name: "a"})
	g.AddNode(Node{UID: b, Kind: "Pod", Name: "b"})
	g.AddEdge(a, b, RelSelects)

	if out := g.Out(a); len(out) != 1 || out[0] != b {
		t.Errorf("Out(a) = %v, want [b]", out)
	}
	if in := g.In(b); len(in) != 1 || in[0] != a {
		t.Errorf("In(b) = %v, want [a]", in)
	}
}
