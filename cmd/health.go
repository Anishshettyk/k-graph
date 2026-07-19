package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/health"
	"github.com/anishetty/kgraph/internal/query"
)

var (
	readyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	unhealthyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

// healthBadge returns an annotator that appends a health badge to Pod nodes.
func healthBadge(color bool) func(graph.Node) string {
	return func(n graph.Node) string {
		if n.Kind != "Pod" {
			return ""
		}
		healthy, reason := health.Pod(n)
		if healthy {
			return style(color, readyStyle, "[Ready]")
		}
		return style(color, unhealthyStyle, fmt.Sprintf("[UNHEALTHY: %s]", reason))
	}
}

// collectUnhealthyNodes walks a dependency tree and returns the graph.Node for
// every unhealthy Pod, deduplicated by UID. Used by whyCmd for diagnosis.
func collectUnhealthyNodes(root *query.TreeNode) []graph.Node {
	seen := map[string]bool{}
	var out []graph.Node
	var walk func(n *query.TreeNode)
	walk = func(n *query.TreeNode) {
		if n.Node.Kind == "Pod" && !n.Cycle {
			if healthy, _ := health.Pod(n.Node); !healthy {
				key := string(n.Node.UID)
				if !seen[key] {
					seen[key] = true
					out = append(out, n.Node)
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// collectUnhealthy walks a dependency tree and returns a description of every
// unhealthy Pod, deduplicated by node UID.
func collectUnhealthy(root *query.TreeNode) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(n *query.TreeNode)
	walk = func(n *query.TreeNode) {
		if n.Node.Kind == "Pod" && !n.Cycle {
			if healthy, reason := health.Pod(n.Node); !healthy {
				key := string(n.Node.UID)
				if !seen[key] {
					seen[key] = true
					out = append(out, fmt.Sprintf("%s (%s)", describe(n.Node), reason))
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

func style(color bool, s lipgloss.Style, text string) string {
	if !color {
		return text
	}
	return s.Render(text)
}
