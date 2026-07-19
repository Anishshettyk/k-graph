// Package cmd defines the Cobra command tree for the kgraph CLI.
package cmd

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/util/homedir"
)

// Flags shared across all commands.
var (
	kubeconfig  string
	kubeContext string
)

// rootCmd is the base command invoked as `kgraph`.
var rootCmd = &cobra.Command{
	Use:   "kgraph",
	Short: "Local-first Kubernetes knowledge-graph CLI",
	Long: `KGraph builds a deterministic knowledge graph of your cluster from your
kubeconfig and answers why / deps / impact questions via graph traversal.

No AI, no CRDs, no cluster-side install. Read-only access only.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command and returns any error to the caller.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	defaultKubeconfig := ""
	if home := homedir.HomeDir(); home != "" {
		defaultKubeconfig = filepath.Join(home, ".kube", "config")
	}

	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", defaultKubeconfig,
		"path to the kubeconfig file")
	rootCmd.PersistentFlags().StringVar(&kubeContext, "context", "",
		"name of the kubeconfig context to use (defaults to current-context)")

	rootCmd.AddCommand(whyCmd)
	rootCmd.AddCommand(depsCmd)
	rootCmd.AddCommand(impactCmd)
	rootCmd.AddCommand(askCmd)
	rootCmd.AddCommand(contextCmd)

	for _, c := range []*cobra.Command{whyCmd, depsCmd, impactCmd} {
		c.Flags().IntVar(&depthFlag, "depth", 0, "limit traversal depth (0 = unlimited)")
		c.ValidArgsFunction = completeResourceRef
	}
}
