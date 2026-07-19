package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/anishetty/kgraph/internal/collector"
)

var (
	eventsNamespace string
	eventsAll       bool
	eventsCount     int
)

var eventsCmd = &cobra.Command{
	Use:   "events <kind>/<name>",
	Short: "Show Kubernetes events for a resource and its pod descendants",
	Long: `Fetches and displays Kubernetes events for the specified resource
and for every Pod found in its dependency tree.

By default only Warning events are shown. Use --all to include Normal events too.
Events are sorted newest-first.`,
	Args: cobra.ExactArgs(1),
	RunE: runEvents,
}

func runEvents(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	ctxName := kubeContext
	if ctxName == "" {
		cfg, err := loadKubeconfig()
		if err != nil {
			return fmt.Errorf("load kubeconfig: %w", err)
		}
		ctxName = cfg.CurrentContext
	}

	g, err := buildGraph(ctx)
	if err != nil {
		return err
	}
	ref, err := parseRef(args[0])
	if err != nil {
		return err
	}
	node, err := resolveNode(g, ref)
	if err != nil {
		return err
	}

	restConfig, err := collector.RESTConfig(kubeconfig, ctxName)
	if err != nil {
		return fmt.Errorf("build rest config: %w", err)
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create clientset: %w", err)
	}

	// Collect UIDs to fetch events for: the node itself + all pod descendants.
	targets := []struct {
		kind, namespace, name, uid string
	}{
		{node.Kind, node.Namespace, node.Name, string(node.UID)},
	}

	// BFS for pod descendants.
	seen := map[string]bool{string(node.UID): true}
	queue := g.Out(node.UID)
	for len(queue) > 0 {
		uid := queue[0]
		queue = queue[1:]
		if seen[string(uid)] {
			continue
		}
		seen[string(uid)] = true
		n, ok := g.Node(uid)
		if !ok {
			continue
		}
		if n.Kind == "Pod" {
			targets = append(targets, struct{ kind, namespace, name, uid string }{
				n.Kind, n.Namespace, n.Name, string(n.UID),
			})
		}
		queue = append(queue, g.Out(uid)...)
	}

	// Fetch events for each target.
	type eventRow struct {
		age     time.Time
		evtType string
		reason  string
		obj     string
		msg     string
	}
	var rows []eventRow

	ns := eventsNamespace
	if ns == "" {
		ns = node.Namespace
		if ns == "" {
			ns = "default"
		}
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for _, t := range targets {
		selector := fmt.Sprintf("involvedObject.name=%s,involvedObject.namespace=%s", t.name, ns)
		list, listErr := client.CoreV1().Events(ns).List(fetchCtx, metav1.ListOptions{FieldSelector: selector})
		if listErr != nil {
			continue
		}
		for _, ev := range list.Items {
			if !eventsAll && ev.Type != corev1.EventTypeWarning {
				continue
			}
			ts := ev.LastTimestamp.Time
			if ts.IsZero() {
				ts = ev.CreationTimestamp.Time
			}
			rows = append(rows, eventRow{
				age:     ts,
				evtType: ev.Type,
				reason:  ev.Reason,
				obj:     t.kind + "/" + t.name,
				msg:     ev.Message,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].age.After(rows[j].age)
	})
	if eventsCount > 0 && len(rows) > eventsCount {
		rows = rows[:eventsCount]
	}

	// Styles.
	title := lipgloss.NewStyle().Bold(true)
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	normal := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	faint := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	objStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))

	scope := "Warning"
	if eventsAll {
		scope = "All"
	}
	fmt.Fprintf(os.Stdout, "%s\n\n", title.Render(fmt.Sprintf(
		"%s events for %s/%s · context: %s", scope, node.Kind, node.Name, ctxName)))

	if len(rows) == 0 {
		fmt.Fprintf(os.Stdout, "  %s\n", faint.Render("No events found."))
		return nil
	}

	now := time.Now()
	for _, r := range rows {
		age := formatAge(now.Sub(r.age))
		evtStyle := warn
		if r.evtType == "Normal" {
			evtStyle = normal
		}
		// Wrap long messages.
		msg := r.msg
		if len(msg) > 120 {
			msg = msg[:117] + "…"
		}
		fmt.Fprintf(os.Stdout, "  %s  %s  %s  %s\n",
			faint.Render(fmt.Sprintf("%-6s", age)),
			evtStyle.Render(fmt.Sprintf("%-18s", r.reason)),
			objStyle.Render(fmt.Sprintf("%-36s", r.obj)),
			msg)
	}
	fmt.Fprintln(os.Stdout)
	return nil
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func init() {
	eventsCmd.Flags().StringVarP(&eventsNamespace, "namespace", "n", "",
		"namespace to query (default: resource's namespace)")
	eventsCmd.Flags().BoolVar(&eventsAll, "all", true,
		"show all event types (use --all=false for Warning events only)")
	eventsCmd.Flags().IntVar(&eventsCount, "count", 20,
		"maximum number of events to show (0 = unlimited)")
	rootCmd.AddCommand(eventsCmd)
}

// formatAge and strings used by other commands — avoid duplicate if already imported.
var _ = strings.Contains
