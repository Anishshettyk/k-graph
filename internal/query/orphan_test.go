package query

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anishetty/kgraph/internal/graph"
)

func TestOrphanedConfigMapAndSecret(t *testing.T) {
	g := graph.Build(graph.Resources{
		ConfigMaps: []corev1.ConfigMap{
			{ObjectMeta: metav1.ObjectMeta{UID: "cm1", Name: "old-config", Namespace: "default"}},
			{ObjectMeta: metav1.ObjectMeta{UID: "cm2", Name: "active-config", Namespace: "default"}},
		},
		Secrets: []corev1.Secret{
			{ObjectMeta: metav1.ObjectMeta{UID: "sec1", Name: "old-secret", Namespace: "default"}},
		},
		Pods: []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{UID: "p1", Name: "api", Namespace: "default"},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{{
						Name:         "cfg",
						VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "active-config"}}},
					}},
				},
			},
		},
	})

	orphans := Orphaned(g, "")
	kinds := map[string]int{}
	names := map[string]bool{}
	for _, o := range orphans {
		kinds[o.Node.Kind]++
		names[o.Node.Name] = true
	}

	if !names["old-config"] {
		t.Error("expected old-config to be orphaned")
	}
	if names["active-config"] {
		t.Error("active-config should NOT be orphaned — it is mounted by a Pod")
	}
	if !names["old-secret"] {
		t.Error("expected old-secret to be orphaned")
	}
}

func TestOrphanedPVC(t *testing.T) {
	g := graph.Build(graph.Resources{
		PVCs: []corev1.PersistentVolumeClaim{
			{ObjectMeta: metav1.ObjectMeta{UID: "pvc1", Name: "scratch", Namespace: "default"},
				Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}},
			{ObjectMeta: metav1.ObjectMeta{UID: "pvc2", Name: "data", Namespace: "default"},
				Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}},
		},
		Pods: []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{UID: "p1", Name: "worker", Namespace: "default"},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{{
						Name:         "data",
						VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}},
					}},
				},
			},
		},
	})
	orphans := Orphaned(g, "")
	names := map[string]bool{}
	for _, o := range orphans {
		names[o.Node.Name] = true
	}
	if !names["scratch"] {
		t.Error("expected scratch PVC to be orphaned")
	}
	if names["data"] {
		t.Error("data PVC should NOT be orphaned — it is mounted")
	}
}

func TestOrphanedServiceNoMatchingPods(t *testing.T) {
	g := graph.Build(graph.Resources{
		Services: []corev1.Service{
			// Selector matches no Pods and no Ingress routes to it
			{ObjectMeta: metav1.ObjectMeta{UID: "svc1", Name: "ghost", Namespace: "default"},
				Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "ghost"}}},
			// No selector — headless, should not appear
			{ObjectMeta: metav1.ObjectMeta{UID: "svc2", Name: "headless", Namespace: "default"},
				Spec: corev1.ServiceSpec{}},
		},
	})
	orphans := Orphaned(g, "")
	names := map[string]bool{}
	for _, o := range orphans {
		names[o.Node.Name] = true
	}
	if !names["ghost"] {
		t.Error("expected ghost Service to be orphaned")
	}
	if names["headless"] {
		t.Error("headless Service should NOT appear in orphans")
	}
}

func TestOrphanNamespaceFilter(t *testing.T) {
	g := graph.Build(graph.Resources{
		ConfigMaps: []corev1.ConfigMap{
			{ObjectMeta: metav1.ObjectMeta{UID: "cm-prod", Name: "cm", Namespace: "prod"}},
			{ObjectMeta: metav1.ObjectMeta{UID: "cm-dev", Name: "cm", Namespace: "dev"}},
		},
	})
	orphans := Orphaned(g, "prod")
	for _, o := range orphans {
		if o.Node.Namespace != "prod" {
			t.Errorf("namespace filter failed: got namespace %q", o.Node.Namespace)
		}
	}
	if len(orphans) != 1 {
		t.Errorf("expected 1 orphan, got %d", len(orphans))
	}
}
