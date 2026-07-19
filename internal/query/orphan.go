package query

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/anishetty/kgraph/internal/graph"
)

// OrphanedNode is a resource that no running workload references in the graph.
type OrphanedNode struct {
	Node   graph.Node
	Reason string
}

// Orphaned returns resources of interest (ConfigMaps, Secrets, PVCs,
// ServiceAccounts, Services) that are not referenced by any currently-running
// workload. Detection is based solely on graph edges, which reflect live Pod
// relationships.
//
// Note: Deployment/StatefulSet pod templates are not scanned, only live Pods.
// A resource referenced in a template but with no running Pods may appear as
// orphaned. This is intentional — if nothing is actually running that uses it,
// it is a candidate for cleanup.
func Orphaned(g *graph.Graph, namespace string) []OrphanedNode {
	var orphans []OrphanedNode

	for _, n := range g.Nodes() {
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		switch n.Kind {
		case "ConfigMap", "Secret", "ServiceAccount":
			// Orphaned if nothing mounts or uses them (no incoming edges).
			if len(g.In(n.UID)) == 0 {
				reason := noRefReason(n.Kind)
				orphans = append(orphans, OrphanedNode{Node: n, Reason: reason})
			}
		case "PersistentVolumeClaim":
			// Orphaned if no Pod mounts it (incoming RelMounts edge).
			if !hasIncomingRel(g, n.UID, graph.RelMounts) {
				orphans = append(orphans, OrphanedNode{Node: n, Reason: "no Pod has it mounted"})
			}
		case "Service":
			// Orphaned if selector is set but no Pods match (no outgoing
			// RelSelects edges) AND no Ingress routes to it.
			if svc, ok := n.Raw.(*corev1.Service); ok && svc != nil && len(svc.Spec.Selector) > 0 {
				if !hasOutgoingRel(g, n.UID, graph.RelSelects) && !hasIncomingRel(g, n.UID, graph.RelRoutes) {
					orphans = append(orphans, OrphanedNode{
						Node:   n,
						Reason: "selector matches no running Pods and no Ingress routes to it",
					})
				}
			}
		}
	}

	sort.Slice(orphans, func(i, j int) bool {
		ai, aj := orphans[i].Node, orphans[j].Node
		if ai.Kind != aj.Kind {
			return ai.Kind < aj.Kind
		}
		if ai.Namespace != aj.Namespace {
			return ai.Namespace < aj.Namespace
		}
		return ai.Name < aj.Name
	})
	return orphans
}

func hasIncomingRel(g *graph.Graph, uid types.UID, rel graph.Relationship) bool {
	for _, e := range g.InEdges(uid) {
		if e.Rel == rel {
			return true
		}
	}
	return false
}

func hasOutgoingRel(g *graph.Graph, uid types.UID, rel graph.Relationship) bool {
	for _, e := range g.OutEdges(uid) {
		if e.Rel == rel {
			return true
		}
	}
	return false
}

func noRefReason(kind string) string {
	switch kind {
	case "ConfigMap":
		return "no Pod mounts or references it"
	case "Secret":
		return "no Pod mounts or references it"
	case "ServiceAccount":
		return "no Pod uses it"
	default:
		return "not referenced by any resource"
	}
}
