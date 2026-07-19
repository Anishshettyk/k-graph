package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/anishetty/kgraph/internal/checker"
)

var checkNamespace string

var checkCmd = &cobra.Command{
	Use:   "check <file> [file...]",
	Short: "Validate manifest cross-references against the live cluster",
	Long: `Reads one or more Kubernetes YAML manifests (multi-document supported) and
cross-references every ConfigMap, Secret, PVC, ServiceAccount, and Ingress backend
against the live cluster graph.

Blocking findings (✖) indicate references that do not exist in the cluster and
will cause the workload to fail. Use --namespace to specify the namespace used
for resources that omit their own namespace field.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		ns := checkNamespace
		if ns == "" {
			cfg, err := loadKubeconfig()
			if err == nil {
				// Use the default namespace from the context if not overridden.
				if ctx := cfg.Contexts[cfg.CurrentContext]; ctx != nil && ctx.Namespace != "" {
					ns = ctx.Namespace
				}
			}
			if ns == "" {
				ns = "default"
			}
		}

		_, g, err := buildLiveGraph(ctx)
		if err != nil {
			return err
		}

		blocking := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
		found := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		label := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
		title := lipgloss.NewStyle().Bold(true)
		faint := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

		totalBlocking, totalFiles := 0, 0
		for _, path := range args {
			result, err := checker.CheckFile(g, path, ns)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				continue
			}
			totalFiles++

			fmt.Fprintf(os.Stdout, "\n%s\n", title.Render(fmt.Sprintf("checking %s (namespace: %s)", path, ns)))

			if len(result.Findings) == 0 {
				fmt.Fprintf(os.Stdout, "  %s\n", faint.Render("no cross-references found to check"))
				continue
			}

			for _, f := range result.Findings {
				if f.Found {
					extra := ""
					if f.FoundExtra != "" {
						extra = " (" + f.FoundExtra + ")"
					}
					fmt.Fprintf(os.Stdout, "  %s  %-30s %s\n",
						found.Render("●"),
						label.Render(f.RefKind+" "+f.Namespace+"/"+f.RefName)+extra,
						faint.Render("✓  ["+f.YAMLPath+"]"))
				} else {
					fmt.Fprintf(os.Stdout, "  %s  %-30s %s\n",
						blocking.Render("✖"),
						f.RefKind+" "+f.RefName,
						faint.Render("not found in ns "+f.Namespace+"  ["+f.YAMLPath+"]"))
					totalBlocking++
				}
			}
			fmt.Fprintf(os.Stdout, "\n  %s\n", checker.FormatSummary(result))
		}

		if totalFiles > 1 {
			fmt.Fprintf(os.Stdout, "\n%s\n", title.Render(fmt.Sprintf("Total: %d blocking across %d files", totalBlocking, totalFiles)))
		}

		if totalBlocking > 0 {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	checkCmd.Flags().StringVarP(&checkNamespace, "namespace", "n", "",
		"default namespace for resources without an explicit namespace (default: context default)")
	rootCmd.AddCommand(checkCmd)
}
