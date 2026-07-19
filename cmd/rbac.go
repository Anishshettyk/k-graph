package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"

	"github.com/anishetty/kgraph/internal/rbac"
)

var rbacNamespace string

var rbacCmd = &cobra.Command{
	Use:   "rbac <sa/name> <can-verb> <resource> [apiGroup]",
	Short: "Trace whether a ServiceAccount can perform a verb on a resource",
	Long: `Evaluates Roles, ClusterRoles, RoleBindings, and ClusterRoleBindings to
determine whether a ServiceAccount can perform a given action. The analysis is
deterministic — no cluster API calls are made beyond the initial graph build.

Arguments:
  sa/<name>         ServiceAccount to check (use -n to set namespace)
  can-<verb>        Action to check: can-get, can-list, can-create, can-delete,
                    can-update, can-patch, can-watch, can-deletecollection
  <resource>        Kubernetes resource: pods, secrets, configmaps, deployments, etc.
                    May include a subresource: pods/log, deployments/scale
  [apiGroup]        Optional API group (default "" = core). Use "apps" for
                    Deployments, "batch" for Jobs, etc.

The command traces every RoleBinding and ClusterRoleBinding that references the
ServiceAccount and shows which rules (if any) grant the requested permission.

Exit codes:
  0   permission granted
  1   permission denied`,
	Args: cobra.RangeArgs(3, 4),
	RunE: runRBAC,
}

func runRBAC(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Parse sa/<name>.
	saRef := args[0]
	if !strings.HasPrefix(strings.ToLower(saRef), "sa/") &&
		!strings.HasPrefix(strings.ToLower(saRef), "serviceaccount/") {
		return fmt.Errorf("first argument must be sa/<name> or serviceaccount/<name>, got %q", saRef)
	}
	saName := saRef[strings.Index(saRef, "/")+1:]

	// Parse can-<verb>.
	verbArg := strings.ToLower(args[1])
	if !strings.HasPrefix(verbArg, "can-") {
		return fmt.Errorf("second argument must be can-<verb> (e.g. can-get, can-list), got %q", args[1])
	}
	verb := strings.TrimPrefix(verbArg, "can-")

	resource := strings.ToLower(args[2])
	apiGroup := ""
	if len(args) == 4 {
		apiGroup = args[3]
	}

	ns := rbacNamespace
	if ns == "" {
		ns = "default"
	}

	ctxName, res, err := buildResources(ctx)
	if err != nil {
		return err
	}

	// Find the ServiceAccount.
	var sa *corev1.ServiceAccount
	for i := range res.ServiceAccounts {
		s := &res.ServiceAccounts[i]
		if s.Name == saName && s.Namespace == ns {
			sa = s
			break
		}
	}
	if sa == nil {
		return fmt.Errorf("ServiceAccount %q not found in namespace %q", saName, ns)
	}

	result := rbac.Check(*sa, verb, resource, apiGroup, ns,
		res.Roles, res.ClusterRoles, res.RoleBindings, res.ClusterRoleBindings)

	// ── Styles ──────────────────────────────────────────────────────────────
	granted := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	denied := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	section := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	titleSt := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	// ── Header ───────────────────────────────────────────────────────────────
	apiGroupStr := apiGroup
	if apiGroupStr == "" {
		apiGroupStr = "core"
	}
	fmt.Fprintf(os.Stdout, "\n%s\n", titleSt.Render(
		fmt.Sprintf("RBAC check · context: %s", ctxName)))
	fmt.Fprintf(os.Stdout, "  ServiceAccount: %s/%s\n", ns, saName)
	fmt.Fprintf(os.Stdout, "  Action:         %s %s  (apiGroup: %s)\n\n", verb, resource, apiGroupStr)

	// ── Verdict ──────────────────────────────────────────────────────────────
	if result.Verdict == rbac.Granted {
		fmt.Fprintf(os.Stdout, "  %s\n\n", granted.Render("✓ GRANTED"))
	} else {
		fmt.Fprintf(os.Stdout, "  %s\n\n", denied.Render("✖ DENIED"))
	}

	// ── Grant paths ──────────────────────────────────────────────────────────
	if len(result.Paths) > 0 {
		fmt.Fprintf(os.Stdout, "  %s\n", section.Render("Grant paths"))
		for _, p := range result.Paths {
			fmt.Fprintf(os.Stdout, "    %s/%s\n",
				p.BindingKind, titleSt.Render(p.BindingName))
			if p.BindingNS != "" {
				fmt.Fprintf(os.Stdout, "      %s %s\n",
					faint.Render("namespace:"), p.BindingNS)
			}
			fmt.Fprintf(os.Stdout, "      %s %s/%s\n",
				faint.Render("role:"), p.RoleKind, p.RoleName)
			fmt.Fprintf(os.Stdout, "      %s rules[%d] grants %s %s\n",
				granted.Render("✓"), p.RuleIndex, verb, resource)
			fmt.Fprintln(os.Stdout)
		}
	}

	// ── Per-binding evaluation ────────────────────────────────────────────────
	fmt.Fprintf(os.Stdout, "  %s\n", section.Render("Binding evaluation"))
	subjectMatches := 0
	for _, c := range result.Checks {
		if !c.SubjectMatch {
			continue // skip bindings for other SAs to reduce noise
		}
		subjectMatches++
		var verdict string
		switch {
		case c.Grants:
			verdict = granted.Render("✓ grants")
		case !c.RoleFound:
			verdict = warn.Render("? role not found")
		default:
			verdict = denied.Render("✖ no matching rule")
		}
		fmt.Fprintf(os.Stdout, "    %s/%s → %s/%s  %s\n",
			c.BindingKind, c.BindingName, c.RoleKind, c.RoleName, verdict)
	}
	if subjectMatches == 0 {
		fmt.Fprintf(os.Stdout, "    %s\n",
			faint.Render("no RoleBindings or ClusterRoleBindings reference this ServiceAccount"))
	}
	fmt.Fprintln(os.Stdout)

	if result.Verdict == rbac.Denied {
		os.Exit(1)
	}
	return nil
}

func init() {
	rbacCmd.Flags().StringVarP(&rbacNamespace, "namespace", "n", "",
		"namespace of the ServiceAccount (default: \"default\")")
	rootCmd.AddCommand(rbacCmd)
}
