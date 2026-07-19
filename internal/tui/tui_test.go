package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anishetty/kgraph/internal/diagnosis"
	"github.com/anishetty/kgraph/internal/graph"
)

func demoGraph() *graph.Graph {
	return graph.Build(graph.Resources{
		Deployments: []appsv1.Deployment{
			{ObjectMeta: metav1.ObjectMeta{UID: "d1", Name: "frontend", Namespace: "default"}},
		},
		ReplicaSets: []appsv1.ReplicaSet{
			{ObjectMeta: metav1.ObjectMeta{UID: "r1", Name: "frontend-abc", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{UID: "d1"}}}},
		},
		Pods: []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{UID: "p1", Name: "frontend-abc-xyz", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{UID: "r1"}}}},
		},
	})
}

func newTestModel() Model {
	g := demoGraph()
	load := func(string) (*graph.Graph, map[string]Usage, error) { return g, map[string]Usage{}, nil }
	return New(g, map[string]Usage{}, "kind-demo", []string{"kind-demo", "prod"}, load, nil)
}

func send(m Model, msg tea.Msg) Model {
	tm, _ := m.Update(msg)
	return tm.(Model)
}

func TestGraphViewRenders(t *testing.T) {
	m := send(newTestModel(), tea.WindowSizeMsg{Width: 100, Height: 30})

	view := m.View()
	for _, want := range []string{"KGraph", "deps", "frontend", "kind-demo", "jump:"} {
		if !strings.Contains(view, want) {
			t.Errorf("graph view missing %q", want)
		}
	}
}

func TestTabTogglesMode(t *testing.T) {
	m := send(newTestModel(), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = send(m, tea.KeyMsg{Type: tea.KeyTab})

	if !strings.Contains(m.View(), "impact") {
		t.Errorf("expected impact mode after tab, got:\n%s", m.View())
	}
}

func TestContextScreen(t *testing.T) {
	m := send(newTestModel(), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})

	view := m.View()
	if !strings.Contains(view, "switch context") {
		t.Errorf("expected context switcher, got:\n%s", view)
	}
}

func TestCommandFiltersByKind(t *testing.T) {
	m := send(newTestModel(), tea.WindowSizeMsg{Width: 100, Height: 30})

	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	if !m.commanding {
		t.Fatal("expected command mode after ':'")
	}
	for _, r := range "pods" {
		m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.commanding {
		t.Error("should exit command mode after enter")
	}
	if m.kindFilter != "Pod" {
		t.Errorf("kindFilter = %q, want Pod", m.kindFilter)
	}
}

func TestNamespacePickerFiltersResources(t *testing.T) {
	g := graph.Build(graph.Resources{Pods: []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{UID: "p-default", Name: "api", Namespace: "default"}},
		{ObjectMeta: metav1.ObjectMeta{UID: "p-prod", Name: "api", Namespace: "prod"}},
	}})
	load := func(string) (*graph.Graph, map[string]Usage, error) { return g, map[string]Usage{}, nil }
	m := send(New(g, map[string]Usage{}, "kind-demo", nil, load, nil), tea.WindowSizeMsg{Width: 100, Height: 30})

	for _, r := range ":ns" {
		m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.screen != screenNamespace {
		t.Fatalf("screen = %v, want namespace picker", m.screen)
	}
	if !strings.Contains(m.View(), "select namespace") {
		t.Error("expected namespace picker view")
	}
	if got := m.namespaceList.Items()[0].(namespaceItem).Title(); got != "All namespaces" {
		t.Errorf("first namespace option = %q, want All namespaces", got)
	}
	m.namespaceList.Select(2) // All namespaces, default, prod.
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.namespace != "prod" {
		t.Errorf("namespace = %q, want prod", m.namespace)
	}
	if got := len(m.resources.Items()); got != 1 {
		t.Errorf("filtered resources = %d, want 1", got)
	}

	model, _ := m.runCommand(":ns")
	m = model.(Model)
	m.namespaceList.Select(0)
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.namespace != "" {
		t.Errorf("namespace = %q, want all namespaces", m.namespace)
	}
	if got := len(m.resources.Items()); got != 2 {
		t.Errorf("all-namespace resources = %d, want 2", got)
	}
}

func TestProblemsViewShowsScopedDiagnosisAndImpact(t *testing.T) {
	g := graph.Build(graph.Resources{
		Deployments: []appsv1.Deployment{{ObjectMeta: metav1.ObjectMeta{UID: "d1", Name: "api", Namespace: "prod"}}},
		Pods: []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{UID: "p-prod", Name: "api-1", Namespace: "prod", OwnerReferences: []metav1.OwnerReference{{UID: "d1"}}}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
			{ObjectMeta: metav1.ObjectMeta{UID: "p-default", Name: "web-1", Namespace: "default"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		},
	})
	load := func(string) (*graph.Graph, map[string]Usage, error) { return g, map[string]Usage{}, nil }
	m := send(New(g, map[string]Usage{}, "kind-demo", nil, load, nil), tea.WindowSizeMsg{Width: 100, Height: 30})
	m.namespace = "prod"
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})

	if m.screen != screenProblems {
		t.Fatalf("screen = %v, want problems", m.screen)
	}
	if got := len(m.problems.Items()); got != 1 {
		t.Fatalf("problems = %d, want 1", got)
	}
	for _, want := range []string{"KGraph problems", "phase=Pending", "Affected resources", "Deployment"} {
		if !strings.Contains(m.View(), want) {
			t.Errorf("problems view missing %q:\n%s", want, m.View())
		}
	}
	if got := lipgloss.Height(m.View()); got > 30 {
		t.Errorf("problems view height = %d, want at most 30", got)
	}
	if got := lipgloss.Width(m.View()); got > 100 {
		t.Errorf("problems view width = %d, want at most 100", got)
	}
}

func TestPVCPendingAppearsInProblems(t *testing.T) {
	sc := "standard"
	g := graph.Build(graph.Resources{
		PVCs: []corev1.PersistentVolumeClaim{
			{ObjectMeta: metav1.ObjectMeta{UID: "pvc-1", Name: "data", Namespace: "default"},
				Spec:   corev1.PersistentVolumeClaimSpec{StorageClassName: &sc},
				Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending}},
		},
		PVs: []corev1.PersistentVolume{
			{ObjectMeta: metav1.ObjectMeta{UID: "pv-1", Name: "pv-1"}},
		},
	})
	items := problemItems(g, "")
	if len(items) != 1 {
		t.Fatalf("problem items = %d, want 1", len(items))
	}
	pi := items[0].(problemItem)
	if pi.node.Kind != "PersistentVolumeClaim" {
		t.Errorf("kind = %q, want PersistentVolumeClaim", pi.node.Kind)
	}
	if pi.reason != "Pending" {
		t.Errorf("reason = %q, want Pending", pi.reason)
	}
}

func TestServiceNoEndpointsAppearsInProblems(t *testing.T) {
	g := graph.Build(graph.Resources{
		Services: []corev1.Service{
			{ObjectMeta: metav1.ObjectMeta{UID: "svc-1", Name: "api", Namespace: "default"},
				Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}},
			{ObjectMeta: metav1.ObjectMeta{UID: "svc-headless", Name: "headless", Namespace: "default"},
				Spec: corev1.ServiceSpec{Selector: map[string]string{}}},
		},
		// No Pods with app=api, so svc-1 has no selector matches.
	})
	items := problemItems(g, "")
	if len(items) != 1 {
		t.Fatalf("problem items = %d, want 1 (only svc with selector but no Pods)", len(items))
	}
	pi := items[0].(problemItem)
	if pi.node.Kind != "Service" || pi.node.Name != "api" {
		t.Errorf("unexpected problem item: kind=%q name=%q", pi.node.Kind, pi.node.Name)
	}
	if pi.reason != "no matching Pods" {
		t.Errorf("reason = %q, want \"no matching Pods\"", pi.reason)
	}
}

func TestServiceWithMatchingPodsDoesNotAppearInProblems(t *testing.T) {
	g := graph.Build(graph.Resources{
		Services: []corev1.Service{
			{ObjectMeta: metav1.ObjectMeta{UID: "svc-1", Name: "api", Namespace: "default"},
				Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}},
		},
		Pods: []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{UID: "p1", Name: "api-1", Namespace: "default",
				Labels: map[string]string{"app": "api"}},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				}}},
		},
	})
	items := problemItems(g, "")
	if len(items) != 0 {
		t.Errorf("problem items = %d, want 0 (service has matching ready Pod)", len(items))
	}
}

func TestDetailsPanelShowsCounts(t *testing.T) {
	// Wide enough to show the details panel.
	m := send(newTestModel(), tea.WindowSizeMsg{Width: 130, Height: 30})

	view := m.View()
	for _, want := range []string{"Details", "depends on", "depended on"} {
		if !strings.Contains(view, want) {
			t.Errorf("details panel missing %q", want)
		}
	}
}

func TestGraphViewFitsTerminalBounds(t *testing.T) {
	const (
		width  = 130
		height = 30
	)
	m := send(newTestModel(), tea.WindowSizeMsg{Width: width, Height: height})

	if got := lipgloss.Height(m.View()); got > height {
		t.Errorf("rendered view height = %d, want at most %d", got, height)
	}
	if got := lipgloss.Width(m.View()); got > width {
		t.Errorf("rendered view width = %d, want at most %d", got, width)
	}
}

func TestDetailsPanelShowsDiagnosisEvidence(t *testing.T) {
	m := send(newTestModel(), tea.WindowSizeMsg{Width: 130, Height: 30})
	target := m.diagnosisTarget()
	if target == nil {
		t.Fatal("expected deployment to resolve its unhealthy Pod")
	}
	m = send(m, diagnosedMsg{uid: target.UID, report: diagnosis.Report{
		Summary:  "Container api is crash looping",
		Findings: []diagnosis.Finding{{Title: "Container crash loop", Detail: "CrashLoopBackOff: restarting failed container"}},
		Events:   []diagnosis.Event{{Reason: "BackOff", Message: "Back-off restarting failed container api"}},
		Logs:     []diagnosis.Log{{Container: "api", Excerpt: "fatal: database connection refused"}},
	}})

	if !strings.Contains(m.View(), "Why it is faili") {
		t.Error("expected rendered details panel to include the diagnosis heading")
	}
	var section strings.Builder
	m.writeDiagnosis(&section, *target)
	for _, want := range []string{"Why it is failing", "crash looping", "event BackOff", "log api"} {
		if !strings.Contains(section.String(), want) {
			t.Errorf("diagnosis section missing %q:\n%s", want, section.String())
		}
	}
}

func TestAsyncContextSwitch(t *testing.T) {
	m := send(newTestModel(), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}) // open switcher
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})                     // pick current, start async load
	if !m.loading {
		t.Fatal("expected loading state after enter")
	}
	// Deliver the completion message.
	m = send(m, loadedMsg{g: demoGraph(), name: "prod", err: nil})
	if m.loading {
		t.Error("loading should be cleared after loadedMsg")
	}
	if m.currentContext != "prod" {
		t.Errorf("context = %q, want prod", m.currentContext)
	}
}
