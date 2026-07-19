package cmd

import (
	"context"
	"sort"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/diagnosis"
	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/metrics"
	"github.com/anishetty/kgraph/internal/tui"
)

// exploreCmd launches the interactive graph explorer.
var exploreCmd = &cobra.Command{
	Use:     "explore",
	Aliases: []string{"tui", "ui"},
	Short:   "Launch the interactive graph explorer (TUI)",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		// load builds the graph and (best-effort) pod usage for a context in a
		// single collection pass.
		load := func(contextName string) (*graph.Graph, map[string]tui.Usage, error) {
			c, err := collector.New(kubeconfig, contextName)
			if err != nil {
				return nil, nil, err
			}
			res, err := c.Collect(ctx)
			if err != nil {
				return nil, nil, err
			}
			g := graph.Build(res)
			return g, podUsageMap(ctx, contextName, res.Pods), nil
		}

		initialContext := kubeContext
		contexts := []string{}
		if cfg, err := loadKubeconfig(); err == nil {
			if initialContext == "" {
				initialContext = cfg.CurrentContext
			}
			for name := range cfg.Contexts {
				contexts = append(contexts, name)
			}
			sort.Strings(contexts)
		}

		g, usage, err := load(initialContext)
		if err != nil {
			return err
		}
		diagnose := func(ctx context.Context, contextName string, node graph.Node) (diagnosis.Report, error) {
			client, err := diagnosis.New(kubeconfig, contextName)
			if err != nil {
				return diagnosis.Report{}, err
			}
			return client.Diagnose(ctx, node)
		}
		return tui.Run(g, usage, initialContext, contexts, load, diagnose)
	},
}

// podUsageMap fetches live pod usage and joins it with declared limits. It is
// best-effort: if the metrics API is unavailable, it returns an empty map.
func podUsageMap(ctx context.Context, contextName string, pods []corev1.Pod) map[string]tui.Usage {
	out := map[string]tui.Usage{}
	m, err := metrics.New(kubeconfig, contextName)
	if err != nil {
		return out
	}
	usage, err := m.Pods(ctx)
	if err != nil {
		return out
	}

	limByKey := map[string]metrics.PodResources{}
	for i := range pods {
		limByKey[pods[i].Namespace+"/"+pods[i].Name] = metrics.ResourcesOf(&pods[i])
	}
	for _, u := range usage {
		key := u.Namespace + "/" + u.Name
		r := limByKey[key]
		out[key] = tui.Usage{
			CPUMilli:      u.CPUMilli,
			MemBytes:      u.MemBytes,
			CPULimitMilli: r.CPULimitMilli,
			MemLimitBytes: r.MemLimitBytes,
		}
	}
	return out
}

func init() {
	rootCmd.AddCommand(exploreCmd)
}
