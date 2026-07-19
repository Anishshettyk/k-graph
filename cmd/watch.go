package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/health"
)

var (
	watchNamespace    string
	watchInterval     int
	watchProblemsOnly bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Stream live cluster changes as they happen",
	Long: `Periodically rebuilds the cluster graph and streams semantic changes to
stdout. Unlike 'kubectl get -w' which shows raw resource events, watch shows
meaningful topology changes: a Service losing endpoints, a Deployment scaling
down, a Pod phase transitioning.

Use --problems-only to only surface new failures, ignoring scaling events and
rollouts that are progressing normally.`,
	Args: cobra.NoArgs,
	RunE: runWatch,
}

func runWatch(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	ctxName, prev, err := buildLiveGraph(ctx)
	if err != nil {
		return err
	}

	added := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	removed := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	changed := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	bad := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	faint := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	title := lipgloss.NewStyle().Bold(true)

	fmt.Fprintf(os.Stdout, "%s\n", title.Render(
		fmt.Sprintf("Watching cluster · context: %s · interval: %ds · Ctrl+C to stop", ctxName, watchInterval)))
	if watchNamespace != "" {
		fmt.Fprintf(os.Stdout, "  scope: namespace %s\n", watchNamespace)
	}
	if watchProblemsOnly {
		fmt.Fprintf(os.Stdout, "  mode: problems only\n")
	}
	fmt.Fprintln(os.Stdout)

	// Catch Ctrl+C for a clean exit message.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(watchInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sig:
			fmt.Fprintf(os.Stdout, "\n%s\n", faint.Render("watch stopped"))
			return nil
		case <-ticker.C:
			_, curr, buildErr := buildLiveGraph(ctx)
			if buildErr != nil {
				fmt.Fprintf(os.Stdout, "%s %v\n",
					bad.Render("collect error:"), buildErr)
				continue
			}

			ts := time.Now().Format("15:04:05")
			changes := diffGraphs(prev, curr, watchNamespace, watchProblemsOnly)
			if len(changes) > 0 {
				fmt.Fprintf(os.Stdout, "  %s\n", faint.Render(ts))
				for _, c := range changes {
					switch c.kind {
					case "added":
						fmt.Fprintf(os.Stdout, "  %s  %s/%s  %s\n",
							added.Render("ADDED  "), c.resourceKind, c.name, faint.Render(c.detail))
					case "removed":
						fmt.Fprintf(os.Stdout, "  %s  %s/%s  %s\n",
							removed.Render("REMOVED"), c.resourceKind, c.name, faint.Render(c.detail))
					case "changed":
						fmt.Fprintf(os.Stdout, "  %s  %s/%s  %s\n",
							changed.Render("CHANGED"), c.resourceKind, c.name, c.detail)
					case "problem":
						fmt.Fprintf(os.Stdout, "  %s  %s/%s  %s\n",
							bad.Render("PROBLEM"), c.resourceKind, c.name, bad.Render(c.detail))
					}
				}
				fmt.Fprintln(os.Stdout)
			}
			prev = curr
		}
	}
}

type graphChange struct {
	kind         string // added | removed | changed | problem
	resourceKind string
	name         string // namespace/name
	detail       string
}

func diffGraphs(prev, curr *graph.Graph, namespace string, problemsOnly bool) []graphChange {
	prevNodes := nodeMap(prev)
	currNodes := nodeMap(curr)

	var changes []graphChange

	// Added nodes.
	for uid, n := range currNodes {
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		if _, ok := prevNodes[uid]; !ok {
			if problemsOnly && n.Kind != "Pod" {
				continue
			}
			detail := ""
			if n.Kind == "Pod" {
				_, reason := health.Pod(n)
				if reason != "" {
					detail = reason
				}
			}
			changes = append(changes, graphChange{
				kind:         "added",
				resourceKind: n.Kind,
				name:         n.Namespace + "/" + n.Name,
				detail:       detail,
			})
		}
	}

	// Removed nodes.
	for uid, n := range prevNodes {
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		if _, ok := currNodes[uid]; !ok {
			if problemsOnly && n.Kind != "Pod" {
				continue
			}
			changes = append(changes, graphChange{
				kind:         "removed",
				resourceKind: n.Kind,
				name:         n.Namespace + "/" + n.Name,
			})
		}
	}

	// Changed Pods: phase transitions.
	for uid, cn := range currNodes {
		if cn.Kind != "Pod" {
			continue
		}
		if namespace != "" && cn.Namespace != namespace {
			continue
		}
		pn, ok := prevNodes[uid]
		if !ok {
			continue
		}
		prevHealthy, prevReason := health.Pod(pn)
		currHealthy, currReason := health.Pod(cn)
		if prevHealthy == currHealthy {
			continue
		}
		if !currHealthy {
			changes = append(changes, graphChange{
				kind:         "problem",
				resourceKind: "Pod",
				name:         cn.Namespace + "/" + cn.Name,
				detail:       currReason,
			})
		} else if !problemsOnly {
			changes = append(changes, graphChange{
				kind:         "changed",
				resourceKind: "Pod",
				name:         cn.Namespace + "/" + cn.Name,
				detail:       fmt.Sprintf("%s → Ready", prevReason),
			})
		}
	}

	return changes
}

func nodeMap(g *graph.Graph) map[string]graph.Node {
	nodes := g.Nodes()
	m := make(map[string]graph.Node, len(nodes))
	for _, n := range nodes {
		m[string(n.UID)] = n
	}
	return m
}

func init() {
	watchCmd.Flags().StringVarP(&watchNamespace, "namespace", "n", "",
		"limit watch to one namespace (default: all namespaces)")
	watchCmd.Flags().IntVar(&watchInterval, "interval", 10,
		"poll interval in seconds")
	watchCmd.Flags().BoolVar(&watchProblemsOnly, "problems-only", false,
		"only surface new Pod failures, skip scaling events and rollouts")
	rootCmd.AddCommand(watchCmd)
}
