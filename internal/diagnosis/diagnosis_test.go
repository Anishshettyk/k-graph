package diagnosis

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/anishetty/kgraph/internal/graph"
)

func TestDiagnoseCorrelatesCrashLoopAndWarningEvents(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{UID: "pod-1", Name: "payments", Namespace: "default"},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name:         "api",
			RestartCount: 3,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason: "CrashLoopBackOff", Message: "back-off 5m0s restarting failed container=api",
			}},
		}}},
	}
	client := fake.NewSimpleClientset(&corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "payments.1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{UID: pod.UID},
		Type:           corev1.EventTypeWarning,
		Reason:         "BackOff",
		Message:        "Back-off restarting failed container api",
		LastTimestamp:  metav1.NewTime(time.Now()),
	})

	report, err := NewWithClient(client).Diagnose(context.Background(), graph.Node{
		UID: pod.UID, Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, Raw: &pod,
	})
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if !strings.Contains(report.Summary, "crash looping") {
		t.Errorf("summary = %q, want crash-loop explanation", report.Summary)
	}
	if len(report.Findings) != 1 || report.Findings[0].Title != "Container crash loop" {
		t.Errorf("findings = %#v", report.Findings)
	}
	if len(report.Events) != 1 || report.Events[0].Reason != "BackOff" {
		t.Errorf("events = %#v", report.Events)
	}
}

func TestDiagnoseExplainsImagePullAndScheduling(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{UID: "pod-2", Name: "catalog", Namespace: "default"},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "0/3 nodes have insufficient memory",
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "api", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
					Reason: "ImagePullBackOff", Message: "pull access denied",
				}},
			}},
		},
	}

	report, err := NewWithClient(fake.NewSimpleClientset()).Diagnose(context.Background(), graph.Node{
		UID: pod.UID, Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, Raw: &pod,
	})
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if report.Summary != "Pod cannot be scheduled" {
		t.Errorf("summary = %q, want scheduling explanation", report.Summary)
	}
	if len(report.Findings) != 2 {
		t.Fatalf("findings = %#v, want scheduling and image pull findings", report.Findings)
	}
	if report.Findings[1].Title != "Image pull failed" {
		t.Errorf("second finding = %#v", report.Findings[1])
	}
}

func TestDiagnoseExplainsOOMKill(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{UID: "pod-3", Name: "worker", Namespace: "default"},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name: "worker",
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
				Reason: "OOMKilled",
			}},
		}}},
	}
	report, err := NewWithClient(fake.NewSimpleClientset()).Diagnose(context.Background(), graph.Node{
		UID: pod.UID, Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, Raw: &pod,
	})
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if report.Summary != "Container worker was OOM killed" {
		t.Errorf("summary = %q", report.Summary)
	}
	if len(report.Findings) != 1 || report.Findings[0].Title != "Container out of memory" {
		t.Errorf("findings = %#v", report.Findings)
	}
}

func TestDiagnosePVCPending(t *testing.T) {
	sc := "standard"
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{UID: "pvc-1", Name: "data", Namespace: "default"},
		Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: &sc},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
	}
	report, err := NewWithClient(fake.NewSimpleClientset()).Diagnose(context.Background(), graph.Node{
		UID: pvc.UID, Kind: "PersistentVolumeClaim", Namespace: pvc.Namespace, Name: pvc.Name, Raw: &pvc,
	})
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if !strings.Contains(report.Summary, "waiting to be bound") {
		t.Errorf("summary = %q", report.Summary)
	}
	if len(report.Findings) != 1 || report.Findings[0].Title != "Unbound PVC" {
		t.Errorf("findings = %#v", report.Findings)
	}
	if !strings.Contains(report.Findings[0].Detail, "standard") {
		t.Errorf("finding detail missing storageClass: %q", report.Findings[0].Detail)
	}
}

func TestDiagnosePVCLost(t *testing.T) {
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{UID: "pvc-2", Name: "data", Namespace: "default"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimLost},
	}
	report, err := NewWithClient(fake.NewSimpleClientset()).Diagnose(context.Background(), graph.Node{
		UID: pvc.UID, Kind: "PersistentVolumeClaim", Namespace: pvc.Namespace, Name: pvc.Name, Raw: &pvc,
	})
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if report.Summary != "PVC has lost its bound PersistentVolume" {
		t.Errorf("summary = %q", report.Summary)
	}
	if len(report.Findings) != 1 || report.Findings[0].Title != "Lost PVC" {
		t.Errorf("findings = %#v", report.Findings)
	}
}

func TestDiagnoseServiceNoEndpoints(t *testing.T) {
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{UID: "svc-1", Name: "api", Namespace: "default"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
	}
	// Endpoints object exists but has no ready addresses (empty subsets).
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
	}
	report, err := NewWithClient(fake.NewSimpleClientset(endpoints)).Diagnose(context.Background(), graph.Node{
		UID: svc.UID, Kind: "Service", Namespace: svc.Namespace, Name: svc.Name, Raw: &svc,
	})
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if report.Summary != "Service has no ready endpoints" {
		t.Errorf("summary = %q", report.Summary)
	}
	if len(report.Findings) != 1 || report.Findings[0].Title != "No matching Pods" {
		t.Errorf("findings = %#v", report.Findings)
	}
	if !strings.Contains(report.Findings[0].Detail, "app=api") {
		t.Errorf("finding detail missing selector: %q", report.Findings[0].Detail)
	}
}

func TestDiagnoseServiceWithReadyEndpoints(t *testing.T) {
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{UID: "svc-2", Name: "web", Namespace: "default"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "web"}},
	}
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
		}},
	}
	report, err := NewWithClient(fake.NewSimpleClientset(endpoints)).Diagnose(context.Background(), graph.Node{
		UID: svc.UID, Kind: "Service", Namespace: svc.Namespace, Name: svc.Name, Raw: &svc,
	})
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if !report.Empty() {
		t.Errorf("expected empty report for healthy service, got %+v", report)
	}
}
