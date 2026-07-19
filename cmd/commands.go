package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/anishetty/kgraph/internal/diagnosis"
	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/health"
	"github.com/anishetty/kgraph/internal/query"
	"github.com/anishetty/kgraph/internal/renderer"
)

// depthFlag limits how many levels deps/impact/why traverse (0 = unlimited).
var depthFlag int

// whyCmd explains a resource's state by walking its dependencies and flagging
// any Pods that are not Running/Ready, then fetching full diagnosis evidence.
var whyCmd = &cobra.Command{
	Use:   "why <kind>/<name>",
	Short: "Explain why a resource is unhealthy (walks dependencies)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseRef(args[0])
		if err != nil {
			return err
		}
		g, err := buildGraph(cmd.Context())
		if err != nil {
			return err
		}
		node, err := resolveNode(g, ref)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		color := colorEnabled(out)
		tree := query.DependencyTreeDepth(g, node, depthFlag)
		renderer.Tree(out, tree, renderer.Options{Color: color, Annotate: healthBadge(color)})

		unhealthyNodes := collectUnhealthyNodes(tree)
		fmt.Fprintln(out)

		// Build a diagnosis client once — used for pod evidence and for direct
		// diagnosis of Services/PVCs when no pod descendant is found.
		diagClient, diagErr := diagnosis.New(kubeconfig, kubeContext)

		// Styles shared across all findings below.
		sevTitle := lipgloss.NewStyle().Bold(true)
		bad := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
		warn := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
		key := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

		// Track whether we've printed the section header yet.
		headerPrinted := false
		printHeader := func() {
			if !headerPrinted {
				fmt.Fprintln(out, sevTitle.Render("Root cause analysis"))
				headerPrinted = true
			}
		}

		// Directly diagnose the starting node when it is a Service or PVC —
		// these can fail without having any unhealthy Pod descendants.
		directKinds := map[string]bool{"PersistentVolumeClaim": true, "Service": true}
		if directKinds[node.Kind] && diagErr == nil {
			diagCtx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			rep, repErr := diagClient.Diagnose(diagCtx, node)
			cancel()
			if repErr == nil && !rep.Empty() {
				printHeader()
				fmt.Fprintf(out, "\n  %s %s\n", bad.Render("✖"), sevTitle.Render(describe(node)))
				if rep.Summary != "" {
					fmt.Fprintf(out, "  %s %s\n", key.Render("summary:"), bad.Render(rep.Summary))
				}
				for _, f := range rep.Findings {
					fmt.Fprintf(out, "  %s %s — %s\n", warn.Render("  finding:"), f.Title, f.Detail)
				}
				for _, ev := range rep.Events {
					fmt.Fprintf(out, "  %s [%s] %s\n", key.Render("    event:"), ev.Reason, ev.Message)
				}
				fmt.Fprintln(out)
			}
		}

		// For a Service with a selector that matches no Pods, the dep tree is
		// empty — graph edges only exist when Pods match. Surface this directly.
		if node.Kind == "Service" && len(unhealthyNodes) == 0 {
			hasSelectedPods := false
			for _, e := range g.OutEdges(node.UID) {
				if e.Rel == graph.RelSelects {
					hasSelectedPods = true
					break
				}
			}
			if !hasSelectedPods {
				if len(g.Out(node.UID)) == 0 {
					printHeader()
					fmt.Fprintf(out, "\n  %s\n\n",
						bad.Render("✖ Service selector matches no running Pods — traffic is dropping to zero endpoints"))
				}
				return nil
			}
		}

		if len(unhealthyNodes) == 0 {
			if !headerPrinted {
				fmt.Fprintln(out, "No unhealthy pods found in the dependency tree.")
			}
			return nil
		}

		// Diagnose each unhealthy Pod found in the dependency tree.
		printHeader()

		for _, podNode := range unhealthyNodes {
			_, reason := health.Pod(podNode)
			fmt.Fprintf(out, "\n  %s %s\n", bad.Render("✖"), sevTitle.Render(describe(podNode)))
			fmt.Fprintf(out, "  %s %s\n", key.Render("status:"), reason)

			if diagErr != nil {
				fmt.Fprintf(out, "  %s\n", dim.Render("diagnosis unavailable: "+diagErr.Error()))
				continue
			}

			diagCtx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			report, rErr := diagClient.Diagnose(diagCtx, podNode)
			cancel()

			if rErr != nil {
				fmt.Fprintf(out, "  %s\n", dim.Render("evidence unavailable: "+rErr.Error()))
				continue
			}
			if report.Empty() {
				fmt.Fprintf(out, "  %s\n", dim.Render("no additional evidence found"))
				continue
			}

			if report.Summary != "" {
				fmt.Fprintf(out, "  %s %s\n", key.Render("summary:"), bad.Render(report.Summary))
			}
			for _, f := range report.Findings {
				fmt.Fprintf(out, "  %s %s — %s\n",
					warn.Render("  finding:"), f.Title, f.Detail)
			}
			for _, ev := range report.Events {
				fmt.Fprintf(out, "  %s [%s] %s\n",
					key.Render("    event:"), ev.Reason, ev.Message)
			}
			for _, lg := range report.Logs {
				lines := lastLogLines(lg.Excerpt, 3)
				if lines != "" {
					fmt.Fprintf(out, "  %s [%s]\n",
						key.Render("      log:"), lg.Container)
					for _, l := range strings.Split(lines, "\n") {
						if strings.TrimSpace(l) != "" {
							fmt.Fprintf(out, "              %s\n", dim.Render(l))
						}
					}
				}
			}
		}
		return nil
	},
}

func lastLogLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) <= n {
		return strings.TrimSpace(text)
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// depsCmd shows what a resource depends on (forward traversal).
var depsCmd = &cobra.Command{
	Use:   "deps <kind>/<name>",
	Short: "Show what a resource depends on",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseRef(args[0])
		if err != nil {
			return err
		}
		g, err := buildGraph(cmd.Context())
		if err != nil {
			return err
		}
		node, err := resolveNode(g, ref)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		tree := query.DependencyTreeDepth(g, node, depthFlag)
		renderer.Tree(out, tree, renderer.Options{Color: colorEnabled(out)})
		if len(tree.Children) == 0 {
			fmt.Fprintln(out, "  (nothing to depend on)")
		}
		return nil
	},
}

// impactCmd shows what depends on a resource (reverse traversal).
var impactCmd = &cobra.Command{
	Use:   "impact <kind>/<name>",
	Short: "Show what breaks if a resource changes (reverse dependencies)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseRef(args[0])
		if err != nil {
			return err
		}
		g, err := buildGraph(cmd.Context())
		if err != nil {
			return err
		}
		node, err := resolveNode(g, ref)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		tree := query.ImpactTreeDepth(g, node, depthFlag)
		renderer.Tree(out, tree, renderer.Options{Color: colorEnabled(out)})
		if len(tree.Children) == 0 {
			fmt.Fprintln(out, "  (nothing depends on this)")
		}
		return nil
	},
}
