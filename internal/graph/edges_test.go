package graph

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TestBuildRichEdges exercises the full set of edges across workloads, config,
// storage, networking, and scheduling.
func TestBuildRichEdges(t *testing.T) {
	labels := map[string]string{"app": "web"}

	r := Resources{
		StatefulSets: []appsv1.StatefulSet{
			{ObjectMeta: metav1.ObjectMeta{UID: "sts", Name: "db", Namespace: "ns"}},
		},
		CronJobs: []batchv1.CronJob{
			{ObjectMeta: metav1.ObjectMeta{UID: "cron", Name: "report", Namespace: "ns"}},
		},
		Jobs: []batchv1.Job{
			{ObjectMeta: metav1.ObjectMeta{UID: "job", Name: "report-1", Namespace: "ns",
				OwnerReferences: []metav1.OwnerReference{{UID: "cron"}}}},
		},
		Pods: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{UID: "pod", Name: "db-0", Namespace: "ns", Labels: labels,
					OwnerReferences: []metav1.OwnerReference{{UID: "sts"}}},
				Spec: corev1.PodSpec{
					NodeName:           "node-a",
					ServiceAccountName: "web-sa",
					Volumes: []corev1.Volume{
						{Name: "cfg", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "web-config"}}}},
						{Name: "sec", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "web-secret"}}},
						{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data-claim"}}},
					},
				},
			},
			{ObjectMeta: metav1.ObjectMeta{UID: "jobpod", Name: "report-1-x", Namespace: "ns",
				OwnerReferences: []metav1.OwnerReference{{UID: "job"}}}},
		},
		Services: []corev1.Service{
			{ObjectMeta: metav1.ObjectMeta{UID: "svc", Name: "web", Namespace: "ns"},
				Spec: corev1.ServiceSpec{Selector: labels}},
		},
		Ingresses: []networkingv1.Ingress{
			{ObjectMeta: metav1.ObjectMeta{UID: "ing", Name: "web-ing", Namespace: "ns"},
				Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
					IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{Name: "web"}}}},
					}},
				}}}},
		},
		ConfigMaps: []corev1.ConfigMap{{ObjectMeta: metav1.ObjectMeta{UID: "cm", Name: "web-config", Namespace: "ns"}}},
		Secrets:    []corev1.Secret{{ObjectMeta: metav1.ObjectMeta{UID: "sec", Name: "web-secret", Namespace: "ns"}}},
		PVCs: []corev1.PersistentVolumeClaim{
			{ObjectMeta: metav1.ObjectMeta{UID: "pvc", Name: "data-claim", Namespace: "ns"},
				Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "pv-1"}},
		},
		PVs:             []corev1.PersistentVolume{{ObjectMeta: metav1.ObjectMeta{UID: "pv", Name: "pv-1"}}},
		Nodes:           []corev1.Node{{ObjectMeta: metav1.ObjectMeta{UID: "node", Name: "node-a"}}},
		ServiceAccounts: []corev1.ServiceAccount{{ObjectMeta: metav1.ObjectMeta{UID: "sa", Name: "web-sa", Namespace: "ns"}}},
	}

	g := Build(r)
	edges := make(map[Edge]bool)
	for _, e := range g.Edges() {
		edges[e] = true
	}

	want := []Edge{
		{From: "sts", To: "pod", Rel: RelOwns},
		{From: "cron", To: "job", Rel: RelOwns},
		{From: "job", To: "jobpod", Rel: RelOwns},
		{From: "svc", To: "pod", Rel: RelSelects},
		{From: "ing", To: "svc", Rel: RelRoutes},
		{From: "pod", To: "cm", Rel: RelMounts},
		{From: "pod", To: "sec", Rel: RelMounts},
		{From: "pod", To: "pvc", Rel: RelMounts},
		{From: "pvc", To: "pv", Rel: RelBinds},
		{From: "pod", To: "node", Rel: RelScheduled},
		{From: "pod", To: "sa", Rel: RelUses},
	}
	for _, e := range want {
		if !edges[e] {
			t.Errorf("missing edge %s --%s--> %s", e.From, e.Rel, e.To)
		}
	}
}

func TestBuildDedupesEdges(t *testing.T) {
	// A pod that references the same ConfigMap via both a volume and envFrom
	// should produce a single mount edge, not two.
	r := Resources{
		Pods: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{UID: "p", Name: "p", Namespace: "ns"},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{{Name: "c", VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm"}}}}},
				Containers: []corev1.Container{{Name: "main", EnvFrom: []corev1.EnvFromSource{{
					ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm"}}}}}},
			},
		}},
		ConfigMaps: []corev1.ConfigMap{{ObjectMeta: metav1.ObjectMeta{UID: "cm", Name: "cm", Namespace: "ns"}}},
	}

	g := Build(r)
	count := 0
	for _, e := range g.Edges() {
		if e.From == types.UID("p") && e.To == types.UID("cm") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 deduped mount edge, got %d", count)
	}
}
