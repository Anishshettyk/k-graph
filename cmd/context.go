package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// contextCmd lists kubeconfig contexts, optionally filtered by a substring.
var contextCmd = &cobra.Command{
	Use:     "context [filter]",
	Aliases: []string{"ctx"},
	Short:   "List, show, or switch kubeconfig contexts",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadKubeconfig()
		if err != nil {
			return err
		}

		filter := ""
		if len(args) == 1 {
			filter = strings.ToLower(args[0])
		}

		names := make([]string, 0, len(cfg.Contexts))
		for name := range cfg.Contexts {
			if filter == "" || strings.Contains(strings.ToLower(name), filter) {
				names = append(names, name)
			}
		}
		sort.Strings(names)

		out := cmd.OutOrStdout()
		if len(names) == 0 {
			fmt.Fprintf(out, "No contexts match %q\n", filter)
			return nil
		}
		for _, name := range names {
			marker := "  "
			if name == cfg.CurrentContext {
				marker = "* "
			}
			fmt.Fprintf(out, "%s%s\n", marker, name)
		}
		fmt.Fprintf(out, "\n%d context(s); current: %s\n", len(names), cfg.CurrentContext)
		return nil
	},
}

// contextCurrentCmd prints the current context.
var contextCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Print the current kubeconfig context",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadKubeconfig()
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), cfg.CurrentContext)
		return nil
	},
}

// contextUseCmd switches the current context in the kubeconfig file, mirroring
// `kubectl config use-context`. This modifies the local kubeconfig only.
var contextUseCmd = &cobra.Command{
	Use:   "use <context>",
	Short: "Switch the current kubeconfig context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		pathOpts := kubeconfigPathOptions()
		cfg, err := pathOpts.GetStartingConfig()
		if err != nil {
			return fmt.Errorf("load kubeconfig: %w", err)
		}
		if _, ok := cfg.Contexts[name]; !ok {
			return fmt.Errorf("context %q not found; run `kgraph context %s` to search", name, name)
		}
		cfg.CurrentContext = name
		if err := clientcmd.ModifyConfig(pathOpts, *cfg, true); err != nil {
			return fmt.Errorf("update kubeconfig: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Switched to context %q\n", name)
		return nil
	},
}

// kubeconfigPathOptions builds clientcmd path options honoring the global
// --kubeconfig flag.
func kubeconfigPathOptions() *clientcmd.PathOptions {
	pathOpts := clientcmd.NewDefaultPathOptions()
	if kubeconfig != "" {
		pathOpts.LoadingRules.ExplicitPath = kubeconfig
	}
	return pathOpts
}

// loadKubeconfig returns the merged kubeconfig for read-only inspection.
func loadKubeconfig() (*clientcmdapi.Config, error) {
	cfg, err := kubeconfigPathOptions().GetStartingConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	return cfg, nil
}

func init() {
	contextCmd.AddCommand(contextCurrentCmd)
	contextCmd.AddCommand(contextUseCmd)
}
