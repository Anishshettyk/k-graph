package cmd

import (
	"context"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"strings"

	"github.com/anishetty/kgraph/internal/collector"
	rbacv1 "k8s.io/api/rbac/v1"
)

var rbacAuditCmd = &cobra.Command{
	Use:   "rbac-audit",
	Short: "Scan for RBAC overprivilege: wildcard rules, cluster-admin grants, ghost accounts",
	Long: `Scans all Roles, ClusterRoles, RoleBindings, and ClusterRoleBindings for
dangerous privilege patterns:
  CRITICAL — cluster-admin granted to non-system service accounts
  CRITICAL — wildcard verbs on wildcard resources (equivalent to cluster-admin)
  WARNING  — wildcard verbs on named resources
  WARNING  — write access to all resources
  INFO     — service accounts with bindings but no running pods (ghost accounts)`,
	RunE: runRBACRisk,
}

func init() {
	rbacAuditCmd.Flags().StringVarP(&rbacAuditNS, "namespace", "n", "", "Scope to a namespace")
	rootCmd.AddCommand(rbacAuditCmd)
}

var rbacAuditNS string

var (
	auditCritStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	auditWarnStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	auditInfoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	auditOKStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	auditDimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func runRBACRisk(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	c, err := collector.New(kubeconfig, kubeContext)
	if err != nil {
		return err
	}
	res, err := c.Collect(ctx)
	if err != nil {
		return err
	}

	// Build pod usage map
	saUsage := map[string]int{}
	for _, pod := range res.Pods {
		sa := pod.Namespace + "/" + pod.Spec.ServiceAccountName
		if pod.Spec.ServiceAccountName == "" {
			sa = pod.Namespace + "/default"
		}
		saUsage[sa]++
	}

	// Role rules lookup
	type roleKey struct{ kind, ns, name string }
	roleRules := map[roleKey][]rbacv1.PolicyRule{}
	for _, cr := range res.ClusterRoles {
		roleRules[roleKey{"ClusterRole", "", cr.Name}] = cr.Rules
	}
	for _, r2 := range res.Roles {
		roleRules[roleKey{"Role", r2.Namespace, r2.Name}] = r2.Rules
	}

	critical, warning, info := 0, 0, 0

	printFinding := func(level, category, subject, binding, detail, remediation string) {
		var badge string
		switch level {
		case "CRITICAL":
			badge = auditCritStyle.Render("CRITICAL")
			critical++
		case "WARNING":
			badge = auditWarnStyle.Render("WARNING ")
			warning++
		default:
			badge = auditInfoStyle.Render("INFO    ")
			info++
		}
		fmt.Printf("  %s  [%s] %s\n", badge, category, auditDimStyle.Render(binding))
		fmt.Printf("           subject: %s\n", subject)
		fmt.Printf("           %s\n", detail)
		fmt.Printf("           → %s\n\n", remediation)
	}

	auditRulesLocal := func(rules []rbacv1.PolicyRule, subjName, bindingName string) {
		for _, rule := range rules {
			hasWildcardVerb := containsStarLocal(rule.Verbs)
			hasWildcardRes := containsStarLocal(rule.Resources)
			hasWrites := containsAnyLocal(rule.Verbs, "create", "update", "patch", "delete", "*")

			if hasWildcardVerb && hasWildcardRes {
				printFinding("CRITICAL", "wildcard-all", subjName, bindingName,
					"* verbs on * resources — effectively cluster-admin within scope",
					"restrict to specific resources and verbs")
			} else if hasWildcardVerb {
				printFinding("WARNING", "wildcard-verb", subjName, bindingName,
					fmt.Sprintf("* verbs on [%s] — includes future verbs", strings.Join(rule.Resources, ", ")),
					"enumerate only the verbs actually needed")
			} else if hasWildcardRes && hasWrites {
				printFinding("WARNING", "wildcard-res", subjName, bindingName,
					"write access to all resources",
					"enumerate specific resources this workload writes")
			}
		}
	}

	for _, crb := range res.ClusterRoleBindings {
		rules := roleRules[roleKey{"ClusterRole", "", crb.RoleRef.Name}]
		for _, subj := range crb.Subjects {
			if subj.Kind != "ServiceAccount" && subj.Kind != "User" {
				continue
			}
			isSystem := strings.HasPrefix(subj.Name, "system:") ||
				strings.HasPrefix(subj.Namespace, "kube-")

			if crb.RoleRef.Name == "cluster-admin" && !isSystem {
				printFinding("CRITICAL", "cluster-admin",
					fmt.Sprintf("%s/%s", subj.Kind, subj.Name),
					"ClusterRoleBinding/"+crb.Name,
					"has cluster-admin — full control of the entire cluster",
					"bind to a least-privilege ClusterRole scoped to actual needs")
			}
			auditRulesLocal(rules, subj.Kind+"/"+subj.Name, "ClusterRoleBinding/"+crb.Name)

			if subj.Kind == "ServiceAccount" && !isSystem {
				key := subj.Namespace + "/" + subj.Name
				if saUsage[key] == 0 {
					printFinding("INFO", "ghost-account",
						fmt.Sprintf("ServiceAccount/%s (ns=%s)", subj.Name, subj.Namespace),
						"ClusterRoleBinding/"+crb.Name,
						"has binding but no pods currently use this ServiceAccount",
						"delete the binding if this SA is no longer needed")
				}
			}
		}
	}

	for _, rb := range res.RoleBindings {
		if rbacAuditNS != "" && rb.Namespace != rbacAuditNS {
			continue
		}
		var rules []rbacv1.PolicyRule
		if rb.RoleRef.Kind == "ClusterRole" {
			rules = roleRules[roleKey{"ClusterRole", "", rb.RoleRef.Name}]
		} else {
			rules = roleRules[roleKey{"Role", rb.Namespace, rb.RoleRef.Name}]
		}
		for _, subj := range rb.Subjects {
			if subj.Kind != "ServiceAccount" && subj.Kind != "User" {
				continue
			}
			auditRulesLocal(rules, subj.Kind+"/"+subj.Name, fmt.Sprintf("RoleBinding/%s/%s", rb.Namespace, rb.Name))
		}
	}

	if critical == 0 && warning == 0 && info == 0 {
		fmt.Println("  " + auditOKStyle.Render("✓  No RBAC overprivilege findings"))
	} else {
		fmt.Printf("  %s  %s  %s\n",
			auditCritStyle.Render(fmt.Sprintf("%d critical", critical)),
			auditWarnStyle.Render(fmt.Sprintf("%d warning", warning)),
			auditInfoStyle.Render(fmt.Sprintf("%d info", info)),
		)
	}
	return nil
}

func containsStarLocal(items []string) bool {
	for _, s := range items {
		if s == "*" {
			return true
		}
	}
	return false
}

func containsAnyLocal(items []string, targets ...string) bool {
	set := make(map[string]bool, len(targets))
	for _, t := range targets {
		set[t] = true
	}
	for _, s := range items {
		if set[s] {
			return true
		}
	}
	return false
}
