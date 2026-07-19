package rbac

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var apiSA = corev1.ServiceAccount{
	ObjectMeta: metav1.ObjectMeta{Name: "api-worker", Namespace: "default"},
}

func TestGrantedByRole(t *testing.T) {
	roles := []rbacv1.Role{{
		ObjectMeta: metav1.ObjectMeta{Name: "secret-reader", Namespace: "default"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Verbs:     []string{"get", "list"},
			Resources: []string{"secrets"},
		}},
	}}
	rbs := []rbacv1.RoleBinding{{
		ObjectMeta: metav1.ObjectMeta{Name: "api-secret-reader", Namespace: "default"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "api-worker", Namespace: "default"}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "secret-reader"},
	}}
	r := Check(apiSA, "get", "secrets", "", "default", roles, nil, rbs, nil)
	if r.Verdict != Granted {
		t.Errorf("verdict = %v, want Granted", r.Verdict)
	}
	if len(r.Paths) != 1 || r.Paths[0].BindingName != "api-secret-reader" {
		t.Errorf("paths = %+v", r.Paths)
	}
}

func TestDeniedNoBinding(t *testing.T) {
	r := Check(apiSA, "delete", "secrets", "", "default", nil, nil, nil, nil)
	if r.Verdict != Denied {
		t.Errorf("verdict = %v, want Denied", r.Verdict)
	}
	if len(r.Paths) != 0 {
		t.Errorf("expected no grant paths, got %+v", r.Paths)
	}
}

func TestGrantedByClusterRole(t *testing.T) {
	crs := []rbacv1.ClusterRole{{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-reader"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Verbs:     []string{"*"},
			Resources: []string{"pods"},
		}},
	}}
	crbs := []rbacv1.ClusterRoleBinding{{
		ObjectMeta: metav1.ObjectMeta{Name: "api-pod-reader"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "api-worker", Namespace: "default"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "pod-reader"},
	}}
	r := Check(apiSA, "list", "pods", "", "", nil, crs, nil, crbs)
	if r.Verdict != Granted {
		t.Errorf("verdict = %v, want Granted", r.Verdict)
	}
}

func TestWildcardVerb(t *testing.T) {
	crs := []rbacv1.ClusterRole{{
		ObjectMeta: metav1.ObjectMeta{Name: "admin"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"*"},
			Verbs:     []string{"*"},
			Resources: []string{"*"},
		}},
	}}
	crbs := []rbacv1.ClusterRoleBinding{{
		ObjectMeta: metav1.ObjectMeta{Name: "api-admin"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "api-worker", Namespace: "default"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "admin"},
	}}
	r := Check(apiSA, "create", "deployments", "apps", "", nil, crs, nil, crbs)
	if r.Verdict != Granted {
		t.Errorf("verdict = %v, want Granted with wildcard rule", r.Verdict)
	}
}

func TestDeniedWrongVerb(t *testing.T) {
	roles := []rbacv1.Role{{
		ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: "default"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Verbs:     []string{"get", "list"},
			Resources: []string{"configmaps"},
		}},
	}}
	rbs := []rbacv1.RoleBinding{{
		ObjectMeta: metav1.ObjectMeta{Name: "api-reader", Namespace: "default"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "api-worker", Namespace: "default"}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "reader"},
	}}
	// Can read but not delete.
	r := Check(apiSA, "delete", "configmaps", "", "default", roles, nil, rbs, nil)
	if r.Verdict != Denied {
		t.Errorf("verdict = %v, want Denied for delete when only get/list granted", r.Verdict)
	}
}

func TestDeniedWrongSubject(t *testing.T) {
	roles := []rbacv1.Role{{
		ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: "default"},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{""}, Verbs: []string{"get"}, Resources: []string{"secrets"}}},
	}}
	// Binding for a different service account.
	rbs := []rbacv1.RoleBinding{{
		ObjectMeta: metav1.ObjectMeta{Name: "other-reader", Namespace: "default"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "other-sa", Namespace: "default"}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "reader"},
	}}
	r := Check(apiSA, "get", "secrets", "", "default", roles, nil, rbs, nil)
	if r.Verdict != Denied {
		t.Errorf("verdict = %v, want Denied — binding is for a different SA", r.Verdict)
	}
}

func TestMultipleGrantPaths(t *testing.T) {
	crs := []rbacv1.ClusterRole{
		{ObjectMeta: metav1.ObjectMeta{Name: "role-a"},
			Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Verbs: []string{"get"}, Resources: []string{"pods"}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "role-b"},
			Rules: []rbacv1.PolicyRule{{APIGroups: []string{"*"}, Verbs: []string{"*"}, Resources: []string{"pods"}}}},
	}
	crbs := []rbacv1.ClusterRoleBinding{
		{ObjectMeta: metav1.ObjectMeta{Name: "binding-a"},
			Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "api-worker", Namespace: "default"}},
			RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "role-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "binding-b"},
			Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "api-worker", Namespace: "default"}},
			RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "role-b"}},
	}
	r := Check(apiSA, "get", "pods", "", "", nil, crs, nil, crbs)
	if r.Verdict != Granted {
		t.Errorf("verdict = %v, want Granted", r.Verdict)
	}
	if len(r.Paths) != 2 {
		t.Errorf("expected 2 grant paths, got %d: %+v", len(r.Paths), r.Paths)
	}
}
