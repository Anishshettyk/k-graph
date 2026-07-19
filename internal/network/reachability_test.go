package network

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func ptr[T any](v T) *T { return &v }

var (
	frontendPod = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "frontend-abc", Namespace: "default",
			Labels: map[string]string{"app": "frontend", "tier": "web"},
		},
	}
	backendPod = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "payments-xyz", Namespace: "default",
			Labels: map[string]string{"app": "payments", "tier": "backend"},
		},
	}
	noNSLabels = map[string]map[string]string{}
	nsLabels   = map[string]map[string]string{
		"default": {"name": "default", "env": "prod"},
		"staging": {"name": "staging", "env": "staging"},
	}
)

func TestNoPolicies_Allowed(t *testing.T) {
	r := Check(frontendPod, backendPod, 8080, corev1.ProtocolTCP, nil, noNSLabels)
	if r.Verdict != Allowed {
		t.Errorf("verdict = %v, want Allowed — no policies should mean open", r.Verdict)
	}
	if !r.Egress.Open || !r.Ingress.Open {
		t.Errorf("expected both directions open, egress=%v ingress=%v", r.Egress.Open, r.Ingress.Open)
	}
}

func TestDenyAllIngress_Blocked(t *testing.T) {
	// A policy that selects the backend Pod with empty ingress (deny all ingress).
	denyAll := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "deny-all", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "payments"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			// No Ingress rules → deny all ingress.
		},
	}
	r := Check(frontendPod, backendPod, 8080, corev1.ProtocolTCP, []networkingv1.NetworkPolicy{denyAll}, noNSLabels)
	if r.Verdict != Blocked {
		t.Errorf("verdict = %v, want Blocked", r.Verdict)
	}
	if r.Egress.Open != true {
		t.Error("egress should still be open (no egress policy)")
	}
	if r.Ingress.Open {
		t.Error("ingress should be restricted")
	}
}

func TestAllowIngressFromPodSelector_Allowed(t *testing.T) {
	allow := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-frontend", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "payments"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "frontend"},
					},
				}},
				Ports: []networkingv1.NetworkPolicyPort{{
					Protocol: ptr(corev1.ProtocolTCP),
					Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8080},
				}},
			}},
		},
	}
	r := Check(frontendPod, backendPod, 8080, corev1.ProtocolTCP, []networkingv1.NetworkPolicy{allow}, noNSLabels)
	if r.Verdict != Allowed {
		t.Errorf("verdict = %v, want Allowed", r.Verdict)
	}
}

func TestAllowIngressWrongPort_Blocked(t *testing.T) {
	// Same policy as above but checking port 443 instead of 8080.
	allow := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-8080", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "payments"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "frontend"}},
				}},
				Ports: []networkingv1.NetworkPolicyPort{{
					Protocol: ptr(corev1.ProtocolTCP),
					Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8080},
				}},
			}},
		},
	}
	r := Check(frontendPod, backendPod, 443, corev1.ProtocolTCP, []networkingv1.NetworkPolicy{allow}, noNSLabels)
	if r.Verdict != Blocked {
		t.Errorf("verdict = %v, want Blocked (wrong port)", r.Verdict)
	}
}

func TestDenyAllEgress_Blocked(t *testing.T) {
	denyEgress := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "deny-egress", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "frontend"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			// No Egress rules → deny all egress.
		},
	}
	r := Check(frontendPod, backendPod, 8080, corev1.ProtocolTCP, []networkingv1.NetworkPolicy{denyEgress}, noNSLabels)
	if r.Verdict != Blocked {
		t.Errorf("verdict = %v, want Blocked (egress denied)", r.Verdict)
	}
	if r.Egress.Open {
		t.Error("egress should be restricted")
	}
}

func TestAllowEgressBySelector_Allowed(t *testing.T) {
	allowEgress := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-payments-egress", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "frontend"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "payments"}},
				}},
			}},
		},
	}
	r := Check(frontendPod, backendPod, 8080, corev1.ProtocolTCP, []networkingv1.NetworkPolicy{allowEgress}, noNSLabels)
	if r.Verdict != Allowed {
		t.Errorf("verdict = %v, want Allowed", r.Verdict)
	}
}

func TestCrossNamespaceSelector_Allowed(t *testing.T) {
	// Cross-namespace: backend is in "default", frontend is also in "default",
	// but the policy uses a namespaceSelector targeting env=prod.
	allow := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-prod-ns", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "payments"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "prod"},
					},
				}},
			}},
		},
	}
	// "default" namespace has env=prod in nsLabels.
	r := Check(frontendPod, backendPod, 8080, corev1.ProtocolTCP,
		[]networkingv1.NetworkPolicy{allow}, nsLabels)
	if r.Verdict != Allowed {
		t.Errorf("verdict = %v, want Allowed — default has env=prod label", r.Verdict)
	}
}

func TestCrossNamespaceSelector_Blocked(t *testing.T) {
	// Same policy but source Pod is in "staging" (env=staging, not prod).
	stagingFrontend := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "frontend-staging", Namespace: "staging",
			Labels: map[string]string{"app": "frontend"},
		},
	}
	allow := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-prod-ns", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "payments"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "prod"},
					},
				}},
			}},
		},
	}
	r := Check(stagingFrontend, backendPod, 8080, corev1.ProtocolTCP,
		[]networkingv1.NetworkPolicy{allow}, nsLabels)
	if r.Verdict != Blocked {
		t.Errorf("verdict = %v, want Blocked — staging does not have env=prod", r.Verdict)
	}
}

func TestAllowAnyPort_Allowed(t *testing.T) {
	// Rule with no port restriction allows all ports.
	allow := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-all-ports", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "payments"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "frontend"},
					},
				}},
				// No Ports → all ports allowed.
			}},
		},
	}
	r := Check(frontendPod, backendPod, 9999, corev1.ProtocolTCP, []networkingv1.NetworkPolicy{allow}, noNSLabels)
	if r.Verdict != Allowed {
		t.Errorf("verdict = %v, want Allowed on any port", r.Verdict)
	}
}
