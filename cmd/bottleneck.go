package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"

	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/metrics"
)

var bottleneckNamespace string

var bottleneckCmd = &cobra.Command{
	Use:   "bottleneck",
	Short: "Detect CPU/memory bottlenecks and surface scaling recommendations",
	Long: `Fetches live metrics from the metrics-server, cross-references them with
declared resource limits, and identifies pods that are near or over their limits.

Severity levels:
  over     — pod has exceeded its limit (CPU throttling / OOM imminent)
  critical — ≥90 % of limit
  warning  — ≥75 % of limit

Pods without limits are always listed — they are invisible to autoscalers and
cannot be monitored for bottlenecks.`,
	Args: cobra.NoArgs,
	RunE: runBottleneck,
}

func runBottleneck(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	ctxName, res, err := buildResources(ctx)
	if err != nil {
		return err
	}

	mc, mcErr := metrics.New(kubeconfig, ctxName)
	var usage []metrics.PodUsage
	if mcErr == nil {
		fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		usage, _ = mc.Pods(fetchCtx)
		cancel()
	}

	g := graph.Build(res)
	_ = g

	usageMap := make(map[string]metrics.PodUsage, len(usage))
	for _, u := range usage {
		usageMap[u.Namespace+"/"+u.Name] = u
	}

	// Styles
	title := lipgloss.NewStyle().Bold(true)
	overSt := lipgloss.NewStyle().Foreground(lipgloss.Color("197")).Bold(true)
	critSt := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	warnSt := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	okSt := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	keySt := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))

	ns := bottleneckNamespace
	scope := "all namespaces"
	if ns != "" {
		scope = "namespace " + ns
	}
	fmt.Fprintf(os.Stdout, "%s\n\n", title.Render(fmt.Sprintf("Bottleneck Report · %s · context: %s", scope, ctxName)))

	if mcErr != nil {
		fmt.Fprintf(os.Stdout, "  %s\n\n", dim.Render("metrics-server not available — showing limit analysis only"))
	}

	type podReport struct {
		namespace string
		name      string
		cpuPct    float64
		memPct    float64
		cpuSev    string
		memSev    string
		noLimits  bool
		cpuRec    string
		memRec    string
	}

	var over, critical, warning, noLimit []podReport

	for _, pod := range res.Pods {
		if ns != "" && pod.Namespace != ns {
			continue
		}
		var cpuLim, cpuReq, memLim, memReq int64
		for _, c := range pod.Spec.Containers {
			if v := c.Resources.Limits.Cpu(); v != nil {
				cpuLim += v.MilliValue()
			}
			if v := c.Resources.Requests.Cpu(); v != nil {
				cpuReq += v.MilliValue()
			}
			if v := c.Resources.Limits.Memory(); v != nil {
				memLim += v.Value()
			}
			if v := c.Resources.Requests.Memory(); v != nil {
				memReq += v.Value()
			}
		}
		u := usageMap[pod.Namespace+"/"+pod.Name]
		r := podReport{namespace: pod.Namespace, name: pod.Name, noLimits: cpuLim == 0 && memLim == 0}
		if cpuLim > 0 {
			r.cpuPct = float64(u.CPUMilli) * 100 / float64(cpuLim)
			r.cpuSev = sevLabel(r.cpuPct)
		}
		if memLim > 0 {
			r.memPct = float64(u.MemBytes) * 100 / float64(memLim)
			r.memSev = sevLabel(r.memPct)
		}
		if r.noLimits {
			noLimit = append(noLimit, r)
			continue
		}
		if r.cpuSev == "over" || r.memSev == "over" {
			over = append(over, r)
		} else if r.cpuSev == "critical" || r.memSev == "critical" {
			critical = append(critical, r)
		} else if r.cpuSev == "warning" || r.memSev == "warning" {
			warning = append(warning, r)
		}
	}

	printGroup := func(label string, st lipgloss.Style, reps []podReport) {
		if len(reps) == 0 {
			return
		}
		fmt.Fprintf(os.Stdout, "  %s\n", st.Render(label))
		for _, r := range reps {
			fmt.Fprintf(os.Stdout, "    %s\n", keySt.Render(r.namespace+"/"+r.name))
			if r.cpuPct > 0 {
				bar := pctBar(r.cpuPct, 20)
				fmt.Fprintf(os.Stdout, "      CPU  %s %5.1f%%\n", bar, r.cpuPct)
			}
			if r.memPct > 0 {
				bar := pctBar(r.memPct, 20)
				fmt.Fprintf(os.Stdout, "      MEM  %s %5.1f%%\n", bar, r.memPct)
			}
			if r.cpuSev == "over" || r.cpuSev == "critical" {
				fmt.Fprintf(os.Stdout, "      %s increase CPU limit or add replicas\n", dim.Render("rec:"))
			}
			if r.memSev == "over" || r.memSev == "critical" {
				fmt.Fprintf(os.Stdout, "      %s increase memory limit or check for leak\n", dim.Render("rec:"))
			}
		}
		fmt.Fprintln(os.Stdout)
	}

	if len(over)+len(critical)+len(warning)+len(noLimit) == 0 {
		fmt.Fprintf(os.Stdout, "  %s\n", okSt.Render("✓ No bottlenecks detected"))
		return nil
	}

	printGroup("✖ OVER LIMIT", overSt, over)
	printGroup("● CRITICAL (≥90%)", critSt, critical)
	printGroup("⚠ WARNING (≥75%)", warnSt, warning)

	if len(noLimit) > 0 {
		fmt.Fprintf(os.Stdout, "  %s\n", dim.Render("◌ NO LIMITS SET (invisible to autoscalers)"))
		for _, r := range noLimit {
			fmt.Fprintf(os.Stdout, "    %s\n", dim.Render(r.namespace+"/"+r.name))
		}
		fmt.Fprintln(os.Stdout)
	}

	return nil
}

func sevLabel(pct float64) string {
	switch {
	case pct > 100:
		return "over"
	case pct >= 90:
		return "critical"
	case pct >= 75:
		return "warning"
	default:
		return "ok"
	}
}

func pctBar(pct float64, width int) string {
	filled := int(pct * float64(width) / 100)
	if filled > width {
		filled = width
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	var st lipgloss.Style
	switch {
	case pct > 100:
		st = lipgloss.NewStyle().Foreground(lipgloss.Color("197"))
	case pct >= 90:
		st = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	case pct >= 75:
		st = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	default:
		st = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	}
	return st.Render(bar)
}

// Suppress unused import — corev1 used indirectly via res.Pods.
var _ corev1.Pod

func init() {
	bottleneckCmd.Flags().StringVarP(&bottleneckNamespace, "namespace", "n", "", "limit scan to one namespace")
	rootCmd.AddCommand(bottleneckCmd)
}
