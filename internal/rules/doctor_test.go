package rules

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anishetty/kgraph/internal/graph"
)

func ptr32(i int32) *int32    { return &i }
func ptrBool(b bool) *bool    { return &b }
func ptrInt64(i int64) *int64 { return &i }
func ptrStr(s string) *string { return &s }

func TestDoctorSingleReplicaDeployment(t *testing.T) {
	g := graph.Build(graph.Resources{
		Deployments: []appsv1.Deployment{{
			ObjectMeta: metav1.ObjectMeta{UID: "d1", Name: "api", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr32(1)},
		}},
	})
	findings := Scan(g, "")
	found := false
	for _, f := range findings {
		if f.Kind == "Deployment" && f.Name == "api" && f.Category == CategoryHA {
			found = true
		}
	}
	if !found {
		t.Error("expected single-replica HA finding for api deployment")
	}
}

func TestDoctorMultiReplicaDeploymentNoHAFinding(t *testing.T) {
	g := graph.Build(graph.Resources{
		Deployments: []appsv1.Deployment{{
			ObjectMeta: metav1.ObjectMeta{UID: "d1", Name: "api", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr32(3)},
			Status:     appsv1.DeploymentStatus{Replicas: 3, AvailableReplicas: 3},
		}},
	})
	findings := Scan(g, "")
	for _, f := range findings {
		if f.Kind == "Deployment" && f.Name == "api" && f.Category == CategoryHA {
			t.Errorf("unexpected HA finding for 3-replica deployment: %s", f.Issue)
		}
	}
}

func TestDoctorPrivilegedContainer(t *testing.T) {
	g := graph.Build(graph.Resources{
		Pods: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{UID: "p1", Name: "db", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{UID: "ss1"}}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:            "db",
				SecurityContext: &corev1.SecurityContext{Privileged: ptrBool(true)},
			}}},
		}},
	})
	findings := Scan(g, "")
	found := false
	for _, f := range findings {
		if f.Kind == "Pod" && f.Severity == SeverityBlocking && f.Category == CategorySecurity {
			found = true
		}
	}
	if !found {
		t.Error("expected blocking security finding for privileged container")
	}
}

func TestDoctorRunAsRoot(t *testing.T) {
	uid := int64(0)
	g := graph.Build(graph.Resources{
		Pods: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{UID: "p1", Name: "worker", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{UID: "d1"}}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:            "worker",
				SecurityContext: &corev1.SecurityContext{RunAsUser: &uid},
			}}},
		}},
	})
	findings := Scan(g, "")
	found := false
	for _, f := range findings {
		if f.Severity == SeverityBlocking && f.Category == CategorySecurity {
			found = true
		}
	}
	if !found {
		t.Error("expected blocking security finding for root container")
	}
}

func TestDoctorMissingProbes(t *testing.T) {
	g := graph.Build(graph.Resources{
		Pods: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{UID: "p1", Name: "api", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{UID: "d1"}}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "api",
				// No liveness or readiness probe.
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			}}},
		}},
	})
	findings := Scan(g, "")
	issues := map[string]bool{}
	for _, f := range findings {
		issues[f.Issue] = true
	}
	hasLiveness := false
	hasReadiness := false
	for issue := range issues {
		if containsStr(issue, "liveness") {
			hasLiveness = true
		}
		if containsStr(issue, "readiness") {
			hasReadiness = true
		}
	}
	if !hasLiveness {
		t.Error("expected liveness probe finding")
	}
	if !hasReadiness {
		t.Error("expected readiness probe finding")
	}
}

func TestDoctorNoResourceLimits(t *testing.T) {
	g := graph.Build(graph.Resources{
		Pods: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{UID: "p1", Name: "api", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{UID: "d1"}}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "api",
				// No resource limits.
			}}},
		}},
	})
	findings := Scan(g, "")
	found := false
	for _, f := range findings {
		if containsStr(f.Issue, "resource limits") {
			found = true
		}
	}
	if !found {
		t.Error("expected resource limits finding")
	}
}

func TestDoctorNamespaceFilter(t *testing.T) {
	g := graph.Build(graph.Resources{
		Deployments: []appsv1.Deployment{
			{ObjectMeta: metav1.ObjectMeta{UID: "d1", Name: "api", Namespace: "prod"},
				Spec: appsv1.DeploymentSpec{Replicas: ptr32(1)}},
			{ObjectMeta: metav1.ObjectMeta{UID: "d2", Name: "api", Namespace: "dev"},
				Spec: appsv1.DeploymentSpec{Replicas: ptr32(1)}},
		},
	})
	findings := Scan(g, "prod")
	for _, f := range findings {
		if f.Namespace != "prod" {
			t.Errorf("namespace filter failed: got finding for namespace %q", f.Namespace)
		}
	}
}

func TestDoctorUnprotectedServiceInPolicyNamespace(t *testing.T) {
	g := graph.Build(graph.Resources{
		Services: []corev1.Service{{
			ObjectMeta: metav1.ObjectMeta{UID: "svc1", Name: "api", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
		}},
		Pods: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{UID: "p1", Name: "api-1", Namespace: "default",
				Labels: map[string]string{"app": "api"}},
		}},
		// A NetworkPolicy exists in the namespace but doesn't select the api Pod.
		NetworkPolicies: []networkingv1.NetworkPolicy{{
			ObjectMeta: metav1.ObjectMeta{UID: "np1", Name: "deny-all", Namespace: "default"},
			Spec:       networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "other"}}},
		}},
	})
	findings := Scan(g, "")
	found := false
	for _, f := range findings {
		if f.Category == CategoryNetwork && f.Name == "api" {
			found = true
		}
	}
	if !found {
		t.Error("expected network finding for unprotected service in policy-aware namespace")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
