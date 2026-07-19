// Package rbac implements deterministic RBAC permission tracing over
// Kubernetes Roles, ClusterRoles, RoleBindings, and ClusterRoleBindings.
// It answers "can ServiceAccount X perform verb Y on resource Z?" without
// any cluster API calls — purely from the collected snapshot.
package rbac

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Verdict is the access decision.
type Verdict int

const (
	Granted Verdict = iota
	Denied
)

func (v Verdict) String() string {
	if v == Granted {
		return "GRANTED"
	}
	return "DENIED"
}

// GrantPath is one complete grant chain that permits the requested access.
type GrantPath struct {
	BindingKind string // "RoleBinding" or "ClusterRoleBinding"
	BindingName string
	BindingNS   string
	RoleKind    string // "Role" or "ClusterRole"
	RoleName    string
	RuleIndex   int // index of the matching PolicyRule within the role
}

// BindingCheck is the evaluation result for one RoleBinding or ClusterRoleBinding.
type BindingCheck struct {
	BindingKind  string
	BindingName  string
	BindingNS    string
	RoleKind     string
	RoleName     string
	SubjectMatch bool // binding has a subject for this ServiceAccount
	RoleFound    bool // referenced role exists in collected data
	Grants       bool // at least one rule permits the request
	MatchedRule  int  // index of first matching rule (-1 = none)
}

// Result is the full RBAC permission analysis for one ServiceAccount + verb + resource.
type Result struct {
	Verdict        Verdict
	Verb           string
	Resource       string
	APIGroup       string
	Namespace      string // namespace scope for the check
	ServiceAccount corev1.ServiceAccount
	Paths          []GrantPath    // all grant chains that allow the request
	Checks         []BindingCheck // per-binding evaluation
}

// Check evaluates whether sa can perform verb on resource (in apiGroup) within
// namespace. Pass namespace="" to check cluster-scoped access only.
// Pass apiGroup="" for core resources (pods, services, configmaps, etc.).
func Check(
	sa corev1.ServiceAccount,
	verb, resource, apiGroup, namespace string,
	roles []rbacv1.Role,
	clusterRoles []rbacv1.ClusterRole,
	roleBindings []rbacv1.RoleBinding,
	clusterRoleBindings []rbacv1.ClusterRoleBinding,
) Result {
	r := Result{
		Verb:           verb,
		Resource:       resource,
		APIGroup:       apiGroup,
		Namespace:      namespace,
		ServiceAccount: sa,
	}

	roleMap := make(map[string]*rbacv1.Role, len(roles))
	for i := range roles {
		key := roles[i].Namespace + "/" + roles[i].Name
		roleMap[key] = &roles[i]
	}
	crMap := make(map[string]*rbacv1.ClusterRole, len(clusterRoles))
	for i := range clusterRoles {
		crMap[clusterRoles[i].Name] = &clusterRoles[i]
	}

	// Check RoleBindings (namespace-scoped grants).
	for _, rb := range roleBindings {
		// A RoleBinding must be in the same namespace as the SA (or if SA ns is empty, match on name only).
		if !bindingInScope(rb.Namespace, sa.Namespace, namespace) {
			continue
		}
		check := BindingCheck{
			BindingKind: "RoleBinding",
			BindingName: rb.Name,
			BindingNS:   rb.Namespace,
			RoleKind:    rb.RoleRef.Kind,
			RoleName:    rb.RoleRef.Name,
			MatchedRule: -1,
		}
		check.SubjectMatch = subjectMatchesSA(rb.Subjects, sa)
		if !check.SubjectMatch {
			r.Checks = append(r.Checks, check)
			continue
		}

		var rules []rbacv1.PolicyRule
		switch rb.RoleRef.Kind {
		case "Role":
			key := rb.Namespace + "/" + rb.RoleRef.Name
			if role, ok := roleMap[key]; ok {
				check.RoleFound = true
				rules = role.Rules
			}
		case "ClusterRole":
			if cr, ok := crMap[rb.RoleRef.Name]; ok {
				check.RoleFound = true
				rules = cr.Rules
			}
		}

		idx := matchingRule(rules, verb, resource, apiGroup)
		if idx >= 0 {
			check.Grants = true
			check.MatchedRule = idx
			r.Paths = append(r.Paths, GrantPath{
				BindingKind: "RoleBinding",
				BindingName: rb.Name,
				BindingNS:   rb.Namespace,
				RoleKind:    rb.RoleRef.Kind,
				RoleName:    rb.RoleRef.Name,
				RuleIndex:   idx,
			})
		}
		r.Checks = append(r.Checks, check)
	}

	// Check ClusterRoleBindings (cluster-scoped grants, apply everywhere).
	for _, crb := range clusterRoleBindings {
		check := BindingCheck{
			BindingKind: "ClusterRoleBinding",
			BindingName: crb.Name,
			BindingNS:   "",
			RoleKind:    "ClusterRole",
			RoleName:    crb.RoleRef.Name,
			MatchedRule: -1,
		}
		check.SubjectMatch = subjectMatchesSA(crb.Subjects, sa)
		if !check.SubjectMatch {
			r.Checks = append(r.Checks, check)
			continue
		}

		if cr, ok := crMap[crb.RoleRef.Name]; ok {
			check.RoleFound = true
			idx := matchingRule(cr.Rules, verb, resource, apiGroup)
			if idx >= 0 {
				check.Grants = true
				check.MatchedRule = idx
				r.Paths = append(r.Paths, GrantPath{
					BindingKind: "ClusterRoleBinding",
					BindingName: crb.Name,
					RoleKind:    "ClusterRole",
					RoleName:    crb.RoleRef.Name,
					RuleIndex:   idx,
				})
			}
		}
		r.Checks = append(r.Checks, check)
	}

	// Sort for deterministic output.
	sort.Slice(r.Paths, func(i, j int) bool {
		return r.Paths[i].BindingName < r.Paths[j].BindingName
	})
	sort.Slice(r.Checks, func(i, j int) bool {
		return r.Checks[i].BindingName < r.Checks[j].BindingName
	})

	if len(r.Paths) > 0 {
		r.Verdict = Granted
	} else {
		r.Verdict = Denied
	}
	return r
}

// subjectMatchesSA returns true when any Subject in the list refers to sa.
func subjectMatchesSA(subjects []rbacv1.Subject, sa corev1.ServiceAccount) bool {
	for _, s := range subjects {
		if s.Kind != "ServiceAccount" {
			continue
		}
		if s.Name != sa.Name {
			continue
		}
		// Namespace match: empty subject namespace is treated as the binding's
		// namespace, but we compare against the SA's namespace here.
		if s.Namespace == "" || s.Namespace == sa.Namespace {
			return true
		}
	}
	return false
}

// bindingInScope returns true when rbNS is applicable for a SA in saNS within
// the requested checkNS. RoleBindings apply within their namespace; we check
// that the binding is in the same namespace as the SA.
func bindingInScope(rbNS, saNS, checkNS string) bool {
	if checkNS != "" {
		return rbNS == checkNS
	}
	return rbNS == saNS
}

// matchingRule returns the index of the first PolicyRule that grants
// verb on resource in apiGroup, or -1 if none match.
func matchingRule(rules []rbacv1.PolicyRule, verb, resource, apiGroup string) int {
	for i, rule := range rules {
		if !matchesSlice(rule.APIGroups, apiGroup) {
			continue
		}
		if !matchesSlice(rule.Verbs, verb) {
			continue
		}
		// Resource may include a subresource separated by "/" (e.g. "pods/log").
		// Match on the base resource; subresource matching is optional.
		baseResource := resource
		if idx := strings.Index(resource, "/"); idx >= 0 {
			baseResource = resource[:idx]
		}
		if !matchesSlice(rule.Resources, baseResource) {
			continue
		}
		return i
	}
	return -1
}

// matchesSlice returns true when slice contains value or "*".
func matchesSlice(slice []string, value string) bool {
	for _, s := range slice {
		if s == "*" || s == value {
			return true
		}
	}
	return false
}
