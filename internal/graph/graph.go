// Package graph defines the knowledge-graph model at the heart of KGraph:
// resources become nodes and relationships become directional edges.
package graph

import (
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

// Relationship names the semantic meaning of an edge between two nodes.
type Relationship string

const (
	// RelOwns is an ownerReference-derived edge (e.g. Deployment owns ReplicaSet).
	RelOwns Relationship = "owns"
	// RelSelects is a label-selector-derived edge (e.g. Service selects Pod).
	RelSelects Relationship = "selects"
	// RelReferences is a direct reference edge (e.g. Pod references a Secret).
	RelReferences Relationship = "references"
	// RelRoutes is an Ingress→Service routing edge.
	RelRoutes Relationship = "routes to"
	// RelMounts is a Pod→ConfigMap/Secret/PVC mount or reference edge.
	RelMounts Relationship = "mounts"
	// RelBinds is a PersistentVolumeClaim→PersistentVolume binding edge.
	RelBinds Relationship = "binds"
	// RelScheduled is a Pod→Node scheduling edge.
	RelScheduled Relationship = "scheduled on"
	// RelUses is a Pod→ServiceAccount edge.
	RelUses Relationship = "uses"
	// RelPolicySelects is a NetworkPolicy→Pod edge: the policy governs this Pod.
	RelPolicySelects Relationship = "policy selects"
	// RelGrants is a RoleBinding/ClusterRoleBinding→ServiceAccount edge.
	RelGrants Relationship = "grants"
	// RelRoleRef is a RoleBinding/ClusterRoleBinding→Role/ClusterRole edge.
	RelRoleRef Relationship = "role ref"
)

// Node identifies a single Kubernetes object in the graph. UID is the stable
// identity; Kind/Namespace/Name are for display and lookup. Raw holds the
// original typed object so callers can inspect resource-specific fields.
type Node struct {
	UID       types.UID
	Kind      string
	Namespace string
	Name      string
	Raw       any
}

// Edge is a directional relationship from one node to another.
type Edge struct {
	From types.UID
	To   types.UID
	Rel  Relationship
}

// Graph is a concurrency-safe collection of nodes and edges. Collectors may
// add to it from multiple goroutines, so all access is guarded by a mutex.
type Graph struct {
	mu    sync.RWMutex
	nodes map[types.UID]Node
	edges []Edge
}

// New returns an empty graph ready for use.
func New() *Graph {
	return &Graph{nodes: make(map[types.UID]Node)}
}

// AddNode inserts or replaces a node keyed by its UID.
func (g *Graph) AddNode(n Node) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[n.UID] = n
}

// AddEdge records a directional relationship between two nodes.
func (g *Graph) AddEdge(from, to types.UID, rel Relationship) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.edges = append(g.edges, Edge{From: from, To: to, Rel: rel})
}

// Node returns the node for a UID and whether it was found.
func (g *Graph) Node(uid types.UID) (Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[uid]
	return n, ok
}

// Nodes returns a snapshot copy of all nodes.
func (g *Graph) Nodes() []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n)
	}
	return out
}

// Edges returns a snapshot copy of all edges.
func (g *Graph) Edges() []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]Edge, len(g.edges))
	copy(out, g.edges)
	return out
}

// Out returns the UIDs of nodes directly reachable from uid (forward edges).
func (g *Graph) Out(uid types.UID) []types.UID {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var out []types.UID
	for _, e := range g.edges {
		if e.From == uid {
			out = append(out, e.To)
		}
	}
	return out
}

// In returns the UIDs of nodes that point at uid (reverse edges).
func (g *Graph) In(uid types.UID) []types.UID {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var in []types.UID
	for _, e := range g.edges {
		if e.To == uid {
			in = append(in, e.From)
		}
	}
	return in
}

// OutEdges returns the forward edges originating at uid, preserving the
// relationship label for each. Edges are returned in insertion order.
func (g *Graph) OutEdges(uid types.UID) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var out []Edge
	for _, e := range g.edges {
		if e.From == uid {
			out = append(out, e)
		}
	}
	return out
}

// InEdges returns the reverse edges pointing at uid, preserving the
// relationship label for each. Edges are returned in insertion order.
func (g *Graph) InEdges(uid types.UID) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var in []Edge
	for _, e := range g.edges {
		if e.To == uid {
			in = append(in, e)
		}
	}
	return in
}

// Find returns all nodes whose kind (case-insensitive) and name match. Callers
// pass a canonical kind such as "Deployment". A namespace filter may be empty
// to match across all namespaces.
func (g *Graph) Find(kind, name, namespace string) []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var out []Node
	for _, n := range g.nodes {
		if !strings.EqualFold(n.Kind, kind) || n.Name != name {
			continue
		}
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		out = append(out, n)
	}
	return out
}
