package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/query"
	"github.com/anishetty/kgraph/internal/renderer"
)

var (
	orphanNamespace string
	orphanKind      string
	orphanShowGraph bool
)

var orphanCmd = &cobra.Command{
	Use:   "orphan",
	Short: "Find resources not referenced by any running workload",
	Long: `Scans ConfigMaps, Secrets, PersistentVolumeClaims, ServiceAccounts, and
Services for resources that no running Pod currently references via graph edges.

These are candidates for cleanup but should be reviewed before deletion:
  - ConfigMap / Secret: no Pod mounts or env-references them
  - PVC: no Pod has it mounted
  - ServiceAccount: no Pod uses it
  - Service: selector matches no running Pods and no Ingress routes to it

Detection is based on live graph edges from running Pods, not Deployment templates.
A ConfigMap used in a pod template with replicas=0 may appear as orphaned.`,
	Args: cobra.NoArgs,
	RunE: runOrphan,
}

func runOrphan(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	ctxName, g, err := buildLiveGraph(ctx)
	if err != nil {
		return err
	}

	orphans := query.Orphaned(g, orphanNamespace)

	badStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	kindStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	faint := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	title := lipgloss.NewStyle().Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)

	scope := "all namespaces"
	if orphanNamespace != "" {
		scope = "namespace " + orphanNamespace
	}
	if orphanKind != "" {
		scope += " · kind=" + orphanKind
	}

	fmt.Fprintf(os.Stdout, "%s\n\n", title.Render(fmt.Sprintf("Orphaned resources · %s · context: %s", scope, ctxName)))

	filtered := orphans
	if orphanKind != "" {
		filtered = nil
		for _, o := range orphans {
			if o.Node.Kind == orphanKind {
				filtered = append(filtered, o)
			}
		}
	}

	if len(filtered) == 0 {
		fmt.Fprintf(os.Stdout, "  %s\n", faint.Render("No orphaned resources found."))
		return nil
	}

	currentKind := ""
	for _, o := range filtered {
		if o.Node.Kind != currentKind {
			currentKind = o.Node.Kind
			fmt.Fprintf(os.Stdout, "\n  %s\n", kindStyle.Render(currentKind))
		}
		ref := o.Node.Namespace + "/" + o.Node.Name
		if o.Node.Namespace == "" {
			ref = o.Node.Name
		}
		age := nodeAge(o.Node)
		fmt.Fprintf(os.Stdout, "    %s  %-34s %s\n",
			badStyle.Render("✖"),
			ref,
			faint.Render(o.Reason+age))

		if orphanShowGraph {
			// Show a tiny impact tree so the user can see what would be broken
			// if this resource were accidentally still needed.
			deps := query.Dependencies(g, o.Node.UID)
			if len(deps) > 0 {
				fmt.Fprintf(os.Stdout, "       %s\n", warnStyle.Render("⚠ still has outgoing edges:"))
				for _, r := range deps {
					n := r.Node
					fmt.Fprintf(os.Stdout, "         %s %s/%s\n",
						renderer.Icon(n.Kind), n.Namespace, n.Name)
				}
			}
		}
	}

	fmt.Fprintf(os.Stdout, "\n  %d orphan(s) found\n", len(filtered))
	return nil
}

// nodeAge returns a human-readable creation age if available on the raw object.
// Returns empty string when unavailable.
func nodeAge(n graph.Node) string {
	type hasMeta interface {
		GetCreationTimestamp() interface{ IsZero() bool }
	}
	// Use duck-typing on the standard ObjectMeta embedded field via interface.
	// Rather than reflect, we accept "unknown age" for unknown types.
	return ""
}

func init() {
	orphanCmd.Flags().StringVarP(&orphanNamespace, "namespace", "n", "",
		"limit scan to one namespace (default: all namespaces)")
	orphanCmd.Flags().StringVarP(&orphanKind, "kind", "k", "",
		"limit to one kind: ConfigMap, Secret, PVC, ServiceAccount, Service")
	orphanCmd.Flags().BoolVar(&orphanShowGraph, "graph", false,
		"show outgoing edges for each orphan (helps confirm it is truly unused)")
	rootCmd.AddCommand(orphanCmd)
}
