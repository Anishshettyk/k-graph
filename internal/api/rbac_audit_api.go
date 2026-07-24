package api

// rbac_audit_api.go — /api/rbac-audit: RBAC overprivilege scanner.
//
// Finds the most dangerous RBAC misconfigurations:
//   - cluster-admin bindings to non-system service accounts
//   - Roles/ClusterRoles with wildcard rules (verbs or resources = "*")
//   - Service accounts that have bindings but are used by zero pods (ghost accounts)
//
// None of this requires extra API calls — everything is in the Resources snapshot.

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
)

type RBACRiskLevel string

const (
	RBACCritical RBACRiskLevel = "critical"
	RBACWarning  RBACRiskLevel = "warning"
	RBACInfo     RBACRiskLevel = "info"
)

type RBACRiskCategory string

const (
	RBACClusterAdmin  RBACRiskCategory = "cluster-admin"   // bound to non-system account
	RBACWildcardAll   RBACRiskCategory = "wildcard-all"    // * verbs on * resources
	RBACWildcardVerb  RBACRiskCategory = "wildcard-verb"   // * verbs on named resources
	RBACWildcardRes   RBACRiskCategory = "wildcard-res"    // write on * resources
	RBACGhostAccount  RBACRiskCategory = "ghost-account"   // SA with bindings but no pods
)

type RBACRiskFinding struct {
	Category    RBACRiskCategory `json:"category"`
	Level       RBACRiskLevel    `json:"level"`
	Subject     string           `json:"subject"`
	SubjectKind string           `json:"subjectKind"`
	SubjectNS   string           `json:"subjectNs,omitempty"`
	Binding     string           `json:"binding"`
	BindingKind string           `json:"bindingKind"`
	BindingNS   string           `json:"bindingNs,omitempty"`
	Role        string           `json:"role"`
	RoleKind    string           `json:"roleKind"`
	Detail      string           `json:"detail"`
	Remediation string           `json:"remediation"`
}

type RBACRiskResponse struct {
	Context  string            `json:"context"`
	Findings []RBACRiskFinding `json:"findings"`
	Critical int               `json:"critical"`
	Warning  int               `json:"warning"`
	Info     int               `json:"info"`
}

func (h *Handler) handleRBACRisk(w http.ResponseWriter, r *http.Request) {
	_, ctxName, ns, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	findings := []RBACRiskFinding{}

	// ─── Build set of pods per SA: "ns/name" → pod count ─────────────────────
	saUsage := map[string]int{}
	for _, pod := range res.Pods {
		sa := pod.Namespace + "/" + pod.Spec.ServiceAccountName
		if pod.Spec.ServiceAccountName == "" {
			sa = pod.Namespace + "/default"
		}
		saUsage[sa]++
	}

	// ─── Build role lookup: "ClusterRole/name" or "Role/ns/name" → rules ──────
	type roleKey struct{ kind, ns, name string }
	roleRules := map[roleKey][]rbacv1.PolicyRule{}
	for _, cr := range res.ClusterRoles {
		roleRules[roleKey{"ClusterRole", "", cr.Name}] = cr.Rules
	}
	for _, r2 := range res.Roles {
		roleRules[roleKey{"Role", r2.Namespace, r2.Name}] = r2.Rules
	}

	// ─── Check ClusterRoleBindings ─────────────────────────────────────────────
	for _, crb := range res.ClusterRoleBindings {
		if ns != "" {
			// cluster-scoped, always include unless caller specifically wants a namespace
		}
		rules := roleRules[roleKey{"ClusterRole", "", crb.RoleRef.Name}]
		for _, subj := range crb.Subjects {
			if subj.Kind != "ServiceAccount" && subj.Kind != "User" {
				continue
			}
			isSystem := strings.HasPrefix(subj.Name, "system:") ||
				strings.HasPrefix(subj.Namespace, "kube-") ||
				strings.HasPrefix(subj.Namespace, "kube-system")

			// cluster-admin to non-system account
			if crb.RoleRef.Name == "cluster-admin" && !isSystem {
				findings = append(findings, RBACRiskFinding{
					Category:    RBACClusterAdmin,
					Level:       RBACCritical,
					Subject:     subj.Name,
					SubjectKind: subj.Kind,
					SubjectNS:   subj.Namespace,
					Binding:     crb.Name,
					BindingKind: "ClusterRoleBinding",
					Role:        "cluster-admin",
					RoleKind:    "ClusterRole",
					Detail:      fmt.Sprintf("%s %q has cluster-admin — full control of the entire cluster", subj.Kind, subj.Name),
					Remediation: "Bind to a least-privilege ClusterRole scoped to the workload's actual needs",
				})
			}

			// wildcard rules
			findings = append(findings, auditRules(rules, subj.Name, subj.Kind, subj.Namespace, crb.Name, "ClusterRoleBinding", crb.RoleRef.Name, crb.RoleRef.Kind)...)

			// ghost SA
			if subj.Kind == "ServiceAccount" && !isSystem {
				key := subj.Namespace + "/" + subj.Name
				if saUsage[key] == 0 {
					findings = append(findings, RBACRiskFinding{
						Category:    RBACGhostAccount,
						Level:       RBACInfo,
						Subject:     subj.Name,
						SubjectKind: "ServiceAccount",
						SubjectNS:   subj.Namespace,
						Binding:     crb.Name,
						BindingKind: "ClusterRoleBinding",
						Role:        crb.RoleRef.Name,
						RoleKind:    crb.RoleRef.Kind,
						Detail:      fmt.Sprintf("ServiceAccount %q has a ClusterRoleBinding but no pods currently use it", subj.Name),
						Remediation: "Verify this SA is still needed; if not, delete the binding to reduce attack surface",
					})
				}
			}
		}
	}

	// ─── Check RoleBindings ────────────────────────────────────────────────────
	for _, rb := range res.RoleBindings {
		if ns != "" && rb.Namespace != ns {
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
			findings = append(findings, auditRules(rules, subj.Name, subj.Kind, subj.Namespace, rb.Name, "RoleBinding", rb.RoleRef.Name, rb.RoleRef.Kind)...)
		}
	}

	// Deduplicate and sort: critical first
	seen := map[string]bool{}
	unique := findings[:0:0]
	for _, f := range findings {
		key := string(f.Category) + "|" + f.Subject + "|" + f.Binding
		if !seen[key] {
			seen[key] = true
			unique = append(unique, f)
		}
	}
	sort.Slice(unique, func(i, j int) bool {
		return riskRank(unique[i].Level) > riskRank(unique[j].Level)
	})

	critical, warning, info := 0, 0, 0
	for _, f := range unique {
		switch f.Level {
		case RBACCritical:
			critical++
		case RBACWarning:
			warning++
		default:
			info++
		}
	}

	writeJSON(w, RBACRiskResponse{
		Context:  ctxName,
		Findings: unique,
		Critical: critical,
		Warning:  warning,
		Info:     info,
	})
}

func auditRules(rules []rbacv1.PolicyRule, subjName, subjKind, subjNS, bindingName, bindingKind, roleName, roleKind string) []RBACRiskFinding {
	var out []RBACRiskFinding
	for _, rule := range rules {
		hasWildcardVerb := containsStar(rule.Verbs)
		hasWildcardRes := containsStar(rule.Resources)
		hasWrites := containsAny(rule.Verbs, "create", "update", "patch", "delete", "deletecollection", "*")

		if hasWildcardVerb && hasWildcardRes {
			out = append(out, RBACRiskFinding{
				Category:    RBACWildcardAll,
				Level:       RBACCritical,
				Subject:     subjName, SubjectKind: subjKind, SubjectNS: subjNS,
				Binding:     bindingName, BindingKind: bindingKind, BindingNS: subjNS,
				Role:        roleName, RoleKind: roleKind,
				Detail:      fmt.Sprintf("Rule grants %q all verbs on all resources — effectively cluster-admin within scope", subjName),
				Remediation: "Restrict to specific resources and verbs actually needed",
			})
		} else if hasWildcardVerb {
			out = append(out, RBACRiskFinding{
				Category:    RBACWildcardVerb,
				Level:       RBACWarning,
				Subject:     subjName, SubjectKind: subjKind, SubjectNS: subjNS,
				Binding:     bindingName, BindingKind: bindingKind,
				Role:        roleName, RoleKind: roleKind,
				Detail:      fmt.Sprintf("Rule grants all verbs on [%s] — includes future verbs", strings.Join(rule.Resources, ", ")),
				Remediation: "Enumerate only the verbs this workload actually needs (get, list, watch)",
			})
		} else if hasWildcardRes && hasWrites {
			out = append(out, RBACRiskFinding{
				Category:    RBACWildcardRes,
				Level:       RBACWarning,
				Subject:     subjName, SubjectKind: subjKind, SubjectNS: subjNS,
				Binding:     bindingName, BindingKind: bindingKind,
				Role:        roleName, RoleKind: roleKind,
				Detail:      fmt.Sprintf("Rule grants write access to all resources — %q can create/modify any object", subjName),
				Remediation: "Enumerate specific resources this workload needs to write",
			})
		}
	}
	return out
}

func containsStar(items []string) bool {
	for _, s := range items {
		if s == "*" {
			return true
		}
	}
	return false
}

func containsAny(items []string, targets ...string) bool {
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

func riskRank(l RBACRiskLevel) int {
	switch l {
	case RBACCritical:
		return 2
	case RBACWarning:
		return 1
	default:
		return 0
	}
}
