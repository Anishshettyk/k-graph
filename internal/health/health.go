// Package health contains deterministic health checks over graph nodes. It is
// pure analysis — no cluster access — so it can be reused by both the CLI
// renderer and the interactive TUI.
package health

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/anishetty/kgraph/internal/graph"
)

// Pod reports whether a Pod node is Running and Ready, plus a short reason when
// it is not. Non-Pod nodes are reported healthy with an empty reason.
func Pod(n graph.Node) (healthy bool, reason string) {
	if n.Kind != "Pod" {
		return true, ""
	}
	pod, ok := n.Raw.(*corev1.Pod)
	if !ok || pod == nil {
		return false, "unknown pod state"
	}
	if pod.Status.Phase != corev1.PodRunning {
		return false, fmt.Sprintf("phase=%s", pod.Status.Phase)
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status != corev1.ConditionTrue {
			if c.Reason != "" {
				return false, fmt.Sprintf("not ready: %s", c.Reason)
			}
			return false, "not ready"
		}
	}
	return true, ""
}
