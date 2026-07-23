package cmd

import (
	"context"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/metrics"
)

var rightsizingCmd = &cobra.Command{
	Use:   "rightsizing",
	Short: "Identify over-provisioned and under-provisioned pods",
	Long: `Cross-references actual CPU/memory usage against declared requests.
Requires metrics-server. Flags pods using <20% of their request (waste) and
pods near or above their request (under-provisioned).`,
	RunE: runRightsizing,
}

func init() {
	rightsizingCmd.Flags().StringVarP(&rsNamespace, "namespace", "n", "", "Namespace to scope")
	rootCmd.AddCommand(rightsizingCmd)
}

var rsNamespace string

var (
	rsWasteStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	rsUnderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	rsOKStyle2    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	rsDimStyle2   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

func runRightsizing(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	c, err := collector.New(kubeconfig, kubeContext)
	if err != nil {
		return err
	}
	res, err := c.Collect(ctx)
	if err != nil {
		return err
	}
	_ = graph.Build(res)

	mc, err := metrics.New(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("metrics-server unavailable: %w", err)
	}
	usage, err := mc.Pods(ctx)
	if err != nil {
		return fmt.Errorf("fetch pod metrics: %w", err)
	}

	usageMap := map[string]metrics.PodUsage{}
	for _, u := range usage {
		usageMap[u.Namespace+"/"+u.Name] = u
	}

	found := 0
	for _, pod := range res.Pods {
		if rsNamespace != "" && pod.Namespace != rsNamespace {
			continue
		}
		u := usageMap[pod.Namespace+"/"+pod.Name]

		var cpuReq, memReq int64
		for _, cnt := range pod.Spec.Containers {
			if v := cnt.Resources.Requests.Cpu(); v != nil {
				cpuReq += v.MilliValue()
			}
			if v := cnt.Resources.Requests.Memory(); v != nil {
				memReq += v.Value()
			}
		}

		if cpuReq == 0 && memReq == 0 {
			continue // no requests set — skip
		}

		cpuPct, memPct := -1.0, -1.0
		if cpuReq > 0 {
			cpuPct = float64(u.CPUMilli) * 100 / float64(cpuReq)
		}
		if memReq > 0 {
			memPct = float64(u.MemBytes) * 100 / float64(memReq)
		}

		cpuLabel := rsDimStyle2.Render("no req")
		if cpuReq > 0 {
			cpuLabel = fmt.Sprintf("%dm/%dm (%.0f%%)", u.CPUMilli, cpuReq, cpuPct)
			switch {
			case cpuPct < 20:
				cpuLabel = rsWasteStyle.Render("WASTE  ") + " " + cpuLabel
			case cpuPct >= 85:
				cpuLabel = rsUnderStyle.Render("UNDER  ") + " " + cpuLabel
			default:
				cpuLabel = rsOKStyle2.Render("OK     ") + " " + cpuLabel
			}
		}

		memLabel := rsDimStyle2.Render("no req")
		if memReq > 0 {
			memMB := u.MemBytes / 1024 / 1024
			reqMB := memReq / 1024 / 1024
			memLabel = fmt.Sprintf("%dMi/%dMi (%.0f%%)", memMB, reqMB, memPct)
			switch {
			case memPct < 20:
				memLabel = rsWasteStyle.Render("WASTE  ") + " " + memLabel
			case memPct >= 85:
				memLabel = rsUnderStyle.Render("UNDER  ") + " " + memLabel
			default:
				memLabel = rsOKStyle2.Render("OK     ") + " " + memLabel
			}
		}

		fmt.Printf("  %-50s  CPU: %-40s  MEM: %s\n",
			rsDimStyle2.Render(pod.Namespace+"/"+pod.Name),
			cpuLabel,
			memLabel,
		)
		found++
	}

	if found == 0 {
		fmt.Println("  No pods with resource requests found (or metrics-server returned no data)")
	}
	return nil
}
