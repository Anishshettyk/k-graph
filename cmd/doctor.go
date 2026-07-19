package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/anishetty/kgraph/internal/rules"
)

var doctorNamespace string

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Scan the cluster for proactive health and security risks",
	Long: `Runs a suite of deterministic checks across the cluster graph and flags
resources that are likely to cause outages, security incidents, or operational
problems — before they happen.

Checks cover:
  High Availability   — single-replica Deployments/StatefulSets, under-replicated workloads
  Reliability         — missing liveness/readiness probes, missing resource limits
  Security            — privileged containers, root containers, privilege escalation
  Storage             — PVCs without an explicit StorageClass
  Network             — Services unprotected by NetworkPolicy (in policy-aware namespaces)

Exit codes:
  0   no findings
  1   warnings found
  2   blocking issues found`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	ctxName, g, err := buildLiveGraph(ctx)
	if err != nil {
		return err
	}

	findings := rules.Scan(g, doctorNamespace)

	blocking := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	warning := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	cat := lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	faint := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	title := lipgloss.NewStyle().Bold(true)
	ok := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)

	scope := "all namespaces"
	if doctorNamespace != "" {
		scope = "namespace " + doctorNamespace
	}
	fmt.Fprintf(os.Stdout, "%s\n\n",
		title.Render(fmt.Sprintf("Doctor · %s · context: %s", scope, ctxName)))

	if len(findings) == 0 {
		fmt.Fprintf(os.Stdout, "  %s\n", ok.Render("✓ No issues found."))
		return nil
	}

	// Group by category for readability.
	order := []string{
		rules.CategoryHA,
		rules.CategoryReliability,
		rules.CategorySecurity,
		rules.CategoryStorage,
		rules.CategoryNetwork,
	}
	byCategory := make(map[string][]rules.Finding, len(order))
	for _, f := range findings {
		byCategory[f.Category] = append(byCategory[f.Category], f)
	}

	hasBlocking := false
	for _, category := range order {
		fs := byCategory[category]
		if len(fs) == 0 {
			continue
		}
		fmt.Fprintf(os.Stdout, "  %s\n", cat.Render(category))
		for _, f := range fs {
			sev := warning.Render("⚠")
			if f.Severity == rules.SeverityBlocking {
				sev = blocking.Render("✖")
				hasBlocking = true
			}
			fmt.Fprintf(os.Stdout, "    %s  %s\n", sev, title.Render(f.Resource()))
			fmt.Fprintf(os.Stdout, "          %s\n", f.Issue)
			if f.Detail != "" {
				fmt.Fprintf(os.Stdout, "          %s\n", faint.Render(f.Detail))
			}
			if f.Fix != "" {
				fmt.Fprintf(os.Stdout, "          %s %s\n",
					lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("fix:"),
					faint.Render(f.Fix))
			}
			fmt.Fprintln(os.Stdout)
		}
	}

	blockingCount := 0
	warningCount := 0
	for _, f := range findings {
		if f.Severity == rules.SeverityBlocking {
			blockingCount++
		} else {
			warningCount++
		}
	}
	fmt.Fprintf(os.Stdout, "  %s %s\n",
		blocking.Render(fmt.Sprintf("%d blocking", blockingCount)),
		warning.Render(fmt.Sprintf("%d warnings", warningCount)))

	if hasBlocking {
		os.Exit(2)
	}
	os.Exit(1)
	return nil
}

func init() {
	doctorCmd.Flags().StringVarP(&doctorNamespace, "namespace", "n", "",
		"limit scan to one namespace (default: all namespaces)")
	rootCmd.AddCommand(doctorCmd)
}
