// Package renderer draws KGraph query results as rich terminal output. It turns
// a query tree into a box-drawing graph with optional color, and stays purely
// presentational — no cluster access, no analysis.
package renderer

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/query"
)

// Options controls how a tree is rendered.
type Options struct {
	// Color enables ANSI styling. Callers should set this to false when output
	// is not a terminal.
	Color bool
	// Annotate returns an optional suffix for a node (e.g. a health badge).
	// It may return an empty string. Nil disables annotations.
	Annotate func(graph.Node) string
}

// kindColors maps resource kinds to lipgloss colors for the kind label.
var kindColors = map[string]lipgloss.Color{
	"Deployment":            lipgloss.Color("12"),  // blue
	"StatefulSet":           lipgloss.Color("33"),  // deep blue
	"DaemonSet":             lipgloss.Color("39"),  // sky
	"ReplicaSet":            lipgloss.Color("14"),  // cyan
	"CronJob":               lipgloss.Color("135"), // purple
	"Job":                   lipgloss.Color("141"), // light purple
	"Pod":                   lipgloss.Color("10"),  // green
	"Service":               lipgloss.Color("13"),  // magenta
	"Ingress":               lipgloss.Color("214"), // orange
	"ConfigMap":             lipgloss.Color("109"), // slate
	"Secret":                lipgloss.Color("203"), // red
	"PersistentVolumeClaim": lipgloss.Color("179"), // tan
	"PersistentVolume":      lipgloss.Color("222"), // sand
	"Node":                  lipgloss.Color("244"), // grey
	"ServiceAccount":        lipgloss.Color("108"), // muted green
	"NetworkPolicy":         lipgloss.Color("196"), // bright red — restrictive resource
	"Namespace":             lipgloss.Color("33"),  // blue
}

// kindIcons maps resource kinds to a compact glyph shown before the kind name.
var kindIcons = map[string]string{
	"Deployment":            "◆",
	"StatefulSet":           "▣",
	"DaemonSet":             "▤",
	"ReplicaSet":            "◇",
	"CronJob":               "◷",
	"Job":                   "▸",
	"Pod":                   "●",
	"Service":               "◈",
	"Ingress":               "⇥",
	"ConfigMap":             "▦",
	"Secret":                "🔒",
	"PersistentVolumeClaim": "▥",
	"PersistentVolume":      "▩",
	"Node":                  "▢",
	"ServiceAccount":        "◉",
	"NetworkPolicy":         "⛔",
	"Namespace":             "□",
}

// Icon returns the glyph for a kind, or a dot if unknown.
func Icon(kind string) string {
	if ic, ok := kindIcons[kind]; ok {
		return ic
	}
	return "•"
}

// KindColor returns the lipgloss color registered for a kind.
func KindColor(kind string) (lipgloss.Color, bool) {
	c, ok := kindColors[kind]
	return c, ok
}

var (
	relStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)
	nameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	cycleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)
)

// Tree writes root and its descendants as a box-drawing tree.
func Tree(w io.Writer, root *query.TreeNode, opts Options) {
	fmt.Fprintln(w, nodeLabel(root, opts))
	renderChildren(w, root.Children, "", opts)
}

func renderChildren(w io.Writer, children []*query.TreeNode, prefix string, opts Options) {
	for i, child := range children {
		last := i == len(children)-1

		connector := "├─ "
		childPrefix := prefix + "│  "
		if last {
			connector = "└─ "
			childPrefix = prefix + "   "
		}

		fmt.Fprintf(w, "%s%s%s\n", prefix, connector, nodeLabel(child, opts))
		renderChildren(w, child.Children, childPrefix, opts)
	}
}

// nodeLabel formats a single tree node: "Kind namespace/name (rel) [badge]".
func nodeLabel(n *query.TreeNode, opts Options) string {
	kind := colorKind(n.Node.Kind, opts.Color)
	name := n.Node.Name
	if n.Node.Namespace != "" {
		name = n.Node.Namespace + "/" + n.Node.Name
	}
	if opts.Color {
		name = nameStyle.Render(name)
	}

	var b strings.Builder
	b.WriteString(kind)
	b.WriteString(" ")
	b.WriteString(name)
	if n.Rel != "" {
		rel := fmt.Sprintf(" (%s)", n.Rel)
		if opts.Color {
			rel = relStyle.Render(rel)
		}
		b.WriteString(rel)
	}

	if n.Cycle {
		mark := " ↩ (already shown)"
		if opts.Color {
			mark = cycleStyle.Render(mark)
		}
		b.WriteString(mark)
	}

	if opts.Annotate != nil {
		if badge := opts.Annotate(n.Node); badge != "" {
			b.WriteString(" ")
			b.WriteString(badge)
		}
	}

	return b.String()
}

func colorKind(kind string, color bool) string {
	icon := Icon(kind)
	label := icon + " " + kind
	if !color {
		return label
	}
	c, ok := kindColors[kind]
	if !ok {
		return label
	}
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render(label)
}

// Legend renders a compact color/icon key for the supported resource kinds,
// wrapping across the given width. Handy above a graph so users can decode it.
func Legend(color bool) string {
	order := []string{
		"Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "CronJob", "Job",
		"Pod", "Service", "Ingress", "ConfigMap", "Secret",
		"PersistentVolumeClaim", "PersistentVolume", "Node", "ServiceAccount",
	}
	short := map[string]string{
		"PersistentVolumeClaim": "PVC",
		"PersistentVolume":      "PV",
		"ServiceAccount":        "SA",
	}
	parts := make([]string, 0, len(order))
	for _, k := range order {
		name := k
		if s, ok := short[k]; ok {
			name = s
		}
		entry := Icon(k) + " " + name
		if color {
			if c, ok := kindColors[k]; ok {
				entry = lipgloss.NewStyle().Foreground(c).Render(entry)
			}
		}
		parts = append(parts, entry)
	}
	return strings.Join(parts, "  ")
}
