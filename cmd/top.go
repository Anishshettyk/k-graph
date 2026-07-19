package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/metrics"
)

var (
	topNodes     bool
	topNamespace string
	topLimit     int
	topSortMem   bool
)

var (
	okBadge   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	critBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
)

// topCmd shows live CPU/memory usage against requests/limits and flags
// bottlenecks (near/over limit, or missing limits).
var topCmd = &cobra.Command{
	Use:   "top",
	Short: "Show live CPU/memory usage vs requests/limits, and flag bottlenecks",
	Long: `Show live resource usage (from the metrics API, like kubectl top) alongside
each pod's requests and limits, sorted by utilization, with bottleneck flags:

  🔴 over/near limit (throttling or OOM risk)   🟡 high, or no limit set   🟢 ok

Requires a metrics-server (or equivalent) in the cluster.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if topNodes {
			return runTopNodes(cmd)
		}
		return runTopPods(cmd)
	},
}

func runTopPods(cmd *cobra.Command) error {
	ctx := cmd.Context()

	c, err := collector.New(kubeconfig, kubeContext)
	if err != nil {
		return err
	}
	res, err := c.Collect(ctx)
	if err != nil {
		return err
	}

	usage, err := podUsage(ctx)
	if err != nil {
		return err
	}

	type row struct {
		ns, name       string
		cpu, mem       int64
		cpuLim, memLim int64
		cpuPct, memPct int
		score          int
		status         string
		statusStyle    lipgloss.Style
	}

	specByKey := map[string]*corev1.Pod{}
	for i := range res.Pods {
		p := &res.Pods[i]
		specByKey[p.Namespace+"/"+p.Name] = p
	}

	var rows []row
	for _, u := range usage {
		if topNamespace != "" && u.Namespace != topNamespace {
			continue
		}
		r := row{ns: u.Namespace, name: u.Name, cpu: u.CPUMilli, mem: u.MemBytes}
		if pod, ok := specByKey[u.Namespace+"/"+u.Name]; ok {
			pr := metrics.ResourcesOf(pod)
			r.cpuLim, r.memLim = pr.CPULimitMilli, pr.MemLimitBytes
		}
		r.cpuPct = pctInt(r.cpu, r.cpuLim)
		r.memPct = pctInt(r.mem, r.memLim)
		r.status, r.statusStyle, r.score = classifyUsage(r.cpuLim, r.memLim, r.cpuPct, r.memPct)
		rows = append(rows, r)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		if topSortMem {
			return rows[i].mem > rows[j].mem
		}
		return rows[i].cpu > rows[j].cpu
	})
	if topLimit > 0 && len(rows) > topLimit {
		rows = rows[:topLimit]
	}

	out := cmd.OutOrStdout()
	color := colorEnabled(out)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tPOD\tCPU\tCPU%LIM\tMEM\tMEM%LIM\tSTATUS")
	for _, r := range rows {
		status := r.status
		if color {
			status = r.statusStyle.Render(status)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.ns, r.name, fmtCPU(r.cpu), fmtPct(r.cpuLim, r.cpuPct),
			fmtMem(r.mem), fmtPct(r.memLim, r.memPct), status)
	}
	return tw.Flush()
}

func runTopNodes(cmd *cobra.Command) error {
	ctx := cmd.Context()

	c, err := collector.New(kubeconfig, kubeContext)
	if err != nil {
		return err
	}
	res, err := c.Collect(ctx)
	if err != nil {
		return err
	}

	m, err := metrics.New(kubeconfig, kubeContext)
	if err != nil {
		return err
	}
	usage, err := m.Nodes(ctx)
	if err != nil {
		return metricsHint(err)
	}

	capByName := map[string]corev1.ResourceList{}
	for i := range res.Nodes {
		capByName[res.Nodes[i].Name] = res.Nodes[i].Status.Allocatable
	}

	out := cmd.OutOrStdout()
	color := colorEnabled(out)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NODE\tCPU\tCPU%\tMEM\tMEM%\tSTATUS")

	sort.Slice(usage, func(i, j int) bool { return usage[i].CPUMilli > usage[j].CPUMilli })
	for _, u := range usage {
		var cpuCap, memCap int64
		if alloc, ok := capByName[u.Name]; ok {
			cpuCap = alloc.Cpu().MilliValue()
			memCap = alloc.Memory().Value()
		}
		cpuPct := pctInt(u.CPUMilli, cpuCap)
		memPct := pctInt(u.MemBytes, memCap)
		status, style, _ := classifyUsage(cpuCap, memCap, cpuPct, memPct)
		if color {
			status = style.Render(status)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			u.Name, fmtCPU(u.CPUMilli), fmtPct(cpuCap, cpuPct),
			fmtMem(u.MemBytes), fmtPct(memCap, memPct), status)
	}
	return tw.Flush()
}

// podUsage fetches pod metrics with a friendly error if the metrics API is
// unavailable.
func podUsage(ctx context.Context) ([]metrics.PodUsage, error) {
	m, err := metrics.New(kubeconfig, kubeContext)
	if err != nil {
		return nil, err
	}
	u, err := m.Pods(ctx)
	if err != nil {
		return nil, metricsHint(err)
	}
	return u, nil
}

func metricsHint(err error) error {
	if strings.Contains(err.Error(), "metrics.k8s.io") || strings.Contains(err.Error(), "the server could not find") {
		return fmt.Errorf("%w\n\nmetrics unavailable — a metrics-server must be installed in the cluster (the same requirement as `kubectl top`)", err)
	}
	return err
}

// classifyUsage returns a status label, style, and a severity score used for sorting.
func classifyUsage(cpuLim, memLim int64, cpuPct, memPct int) (string, lipgloss.Style, int) {
	noLimit := cpuLim == 0 || memLim == 0
	worst := cpuPct
	if memPct > worst {
		worst = memPct
	}
	switch {
	case (cpuLim > 0 && cpuPct >= 90) || (memLim > 0 && memPct >= 90):
		return "🔴 over limit", critBadge, 300 + worst
	case (cpuLim > 0 && cpuPct >= 75) || (memLim > 0 && memPct >= 75):
		return "🟡 high", warnBadge, 200 + worst
	case noLimit:
		return "🟡 no limit", warnBadge, 100
	default:
		return "🟢 ok", okBadge, worst
	}
}

func pctInt(used, capacity int64) int {
	if capacity <= 0 {
		return 0
	}
	return int(used * 100 / capacity)
}

func fmtPct(capacity int64, pct int) string {
	if capacity <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d%%", pct)
}

func fmtCPU(milli int64) string {
	if milli == 0 {
		return "0"
	}
	if milli < 1000 {
		return fmt.Sprintf("%dm", milli)
	}
	return fmt.Sprintf("%.2f", float64(milli)/1000)
}

func fmtMem(b int64) string {
	const (
		mi = 1 << 20
		gi = 1 << 30
	)
	switch {
	case b == 0:
		return "0"
	case b >= gi:
		return fmt.Sprintf("%.1fGi", float64(b)/gi)
	default:
		return fmt.Sprintf("%dMi", b/mi)
	}
}

func init() {
	topCmd.Flags().BoolVar(&topNodes, "nodes", false, "show node usage instead of pods")
	topCmd.Flags().StringVarP(&topNamespace, "namespace", "n", "", "filter pods by namespace")
	topCmd.Flags().IntVar(&topLimit, "limit", 20, "show at most N rows (0 = all)")
	topCmd.Flags().BoolVar(&topSortMem, "sort-mem", false, "sort by memory instead of CPU")
	rootCmd.AddCommand(topCmd)
}
