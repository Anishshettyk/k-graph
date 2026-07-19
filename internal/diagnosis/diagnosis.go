// Package diagnosis correlates Pod status, Kubernetes Warning events, and
// bounded container logs into deterministic, explainable failure reports.
package diagnosis

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/graph"
)

const (
	maxEvents         = 3
	maxLogBytes int64 = 8 * 1024
	maxLogLines int64 = 40
)

// Finding is one status-derived explanation of a failure.
type Finding struct {
	Title  string
	Detail string
}

// Event is a relevant Kubernetes Warning event attached to the diagnosed Pod.
type Event struct {
	Reason  string
	Message string
}

// Log is a bounded excerpt from a failing container's previous execution.
type Log struct {
	Container string
	Excerpt   string
}

// Report explains one Pod failure. All evidence is returned from read-only
// Kubernetes APIs and is retained only in memory by the caller.
type Report struct {
	Summary  string
	Findings []Finding
	Events   []Event
	Logs     []Log
}

// Empty reports have no failure evidence to display.
func (r Report) Empty() bool {
	return r.Summary == "" && len(r.Findings) == 0 && len(r.Events) == 0 && len(r.Logs) == 0
}

// Client reads diagnosis evidence using the Kubernetes APIs.
type Client struct {
	client kubernetes.Interface
}

// New constructs a read-only diagnosis client from a kubeconfig context.
func New(kubeconfig, contextName string) (*Client, error) {
	restConfig, err := collector.RESTConfig(kubeconfig, contextName)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}
	return &Client{client: client}, nil
}

// NewWithClient constructs a diagnosis client from a Kubernetes clientset.
// It is primarily useful for tests with a fake client.
func NewWithClient(client kubernetes.Interface) *Client {
	return &Client{client: client}
}

// Diagnose analyzes a graph node and returns deterministic failure evidence.
//
//   - Pod: correlates container status, Kubernetes Warning events, and bounded
//     previous-container logs.
//   - PersistentVolumeClaim: checks the binding phase from the raw object (no
//     API call required).
//   - Service: fetches the cluster Endpoints object to detect a selector that
//     matches no ready Pods.
//   - All other kinds: returns an empty report.
func (c *Client) Diagnose(ctx context.Context, node graph.Node) (Report, error) {
	switch node.Kind {
	case "Pod":
		return c.diagnosePod(ctx, node)
	case "PersistentVolumeClaim":
		return diagnosePVC(node)
	case "Service":
		return c.diagnoseService(ctx, node)
	default:
		return Report{}, nil
	}
}

func (c *Client) diagnosePod(ctx context.Context, node graph.Node) (Report, error) {
	pod, ok := node.Raw.(*corev1.Pod)
	if !ok || pod == nil {
		return Report{}, fmt.Errorf("read pod %s: unsupported raw object", node.Name)
	}

	report, logContainers := statusReport(pod)
	events, err := c.warningEvents(ctx, pod)
	if err != nil {
		return report, fmt.Errorf("list warning events for pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	report.Events = events
	if report.Summary == "" && len(events) > 0 {
		report.Summary = "Kubernetes reported " + events[0].Reason
	}

	for _, container := range logContainers {
		excerpt, err := c.previousLogs(ctx, pod, container)
		if err != nil || excerpt == "" {
			continue
		}
		report.Logs = append(report.Logs, Log{Container: container, Excerpt: excerpt})
	}
	return report, nil
}

// diagnosePVC derives a failure report from the PVC's phase. It requires no
// API call because the phase is available on the raw object already in the graph.
func diagnosePVC(node graph.Node) (Report, error) {
	pvc, ok := node.Raw.(*corev1.PersistentVolumeClaim)
	if !ok || pvc == nil {
		return Report{}, fmt.Errorf("read pvc %s: unsupported raw object", node.Name)
	}
	switch pvc.Status.Phase {
	case corev1.ClaimBound:
		return Report{}, nil
	case corev1.ClaimPending:
		detail := fmt.Sprintf("PVC %s/%s is Pending — no matching PV available or StorageClass has not provisioned one", pvc.Namespace, pvc.Name)
		if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
			detail += fmt.Sprintf(" (storageClass: %s)", *pvc.Spec.StorageClassName)
		}
		return Report{
			Summary:  "PVC is waiting to be bound to a PersistentVolume",
			Findings: []Finding{{Title: "Unbound PVC", Detail: detail}},
		}, nil
	case corev1.ClaimLost:
		return Report{
			Summary: "PVC has lost its bound PersistentVolume",
			Findings: []Finding{{Title: "Lost PVC",
				Detail: fmt.Sprintf("PVC %s/%s is Lost — the bound PV may have been deleted", pvc.Namespace, pvc.Name),
			}},
		}, nil
	default:
		return Report{}, nil
	}
}

// diagnoseService fetches the cluster Endpoints object for a Service that has a
// non-empty selector and returns a report when no ready addresses exist.
func (c *Client) diagnoseService(ctx context.Context, node graph.Node) (Report, error) {
	svc, ok := node.Raw.(*corev1.Service)
	if !ok || svc == nil {
		return Report{}, fmt.Errorf("read service %s: unsupported raw object", node.Name)
	}
	if len(svc.Spec.Selector) == 0 {
		// Headless or external-name services with no selector are intentionally
		// decoupled from Pods and are not diagnosed here.
		return Report{}, nil
	}
	ep, err := c.client.CoreV1().Endpoints(node.Namespace).Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		return Report{}, fmt.Errorf("get endpoints for service %s/%s: %w", node.Namespace, node.Name, err)
	}
	for _, subset := range ep.Subsets {
		if len(subset.Addresses) > 0 {
			return Report{}, nil // healthy: at least one ready address
		}
	}
	selParts := make([]string, 0, len(svc.Spec.Selector))
	for k, v := range svc.Spec.Selector {
		selParts = append(selParts, k+"="+v)
	}
	sort.Strings(selParts)
	return Report{
		Summary: "Service has no ready endpoints",
		Findings: []Finding{{Title: "No matching Pods",
			Detail: "selector " + strings.Join(selParts, ", ") + " — no Pods are running and ready with these labels",
		}},
	}, nil
}

func statusReport(pod *corev1.Pod) (Report, []string) {
	var report Report
	var logContainers []string

	// Only surface conditions that are direct root causes, not downstream
	// symptoms. Ready and ContainersReady are always false when a container
	// is failing — surfacing them adds noise without adding information.
	rootCauseConditions := map[corev1.PodConditionType]bool{
		corev1.PodScheduled:   true,
		corev1.PodInitialized: true,
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Status != corev1.ConditionFalse || condition.Message == "" {
			continue
		}
		if !rootCauseConditions[condition.Type] {
			continue
		}
		title := string(condition.Type)
		if condition.Type == corev1.PodScheduled {
			title = "Scheduling blocked"
			if report.Summary == "" {
				report.Summary = "Pod cannot be scheduled"
			}
		}
		report.Findings = append(report.Findings, Finding{Title: title, Detail: condition.Message})
	}

	inspect := func(statuses []corev1.ContainerStatus) {
		for _, status := range statuses {
			if waiting := status.State.Waiting; waiting != nil && waiting.Reason != "" {
				title, summary := waitingFinding(status.Name, waiting.Reason)
				detail := waiting.Reason
				if waiting.Message != "" {
					detail += ": " + waiting.Message
				}
				report.Findings = append(report.Findings, Finding{Title: title, Detail: detail})
				if report.Summary == "" {
					report.Summary = summary
				}
				if waiting.Reason == "CrashLoopBackOff" {
					logContainers = append(logContainers, status.Name)
				}
			}

			terminated := status.State.Terminated
			if terminated == nil && status.LastTerminationState.Terminated != nil {
				terminated = status.LastTerminationState.Terminated
			}
			if terminated == nil || terminated.Reason == "" {
				continue
			}
			title, summary := terminatedFinding(status.Name, terminated.Reason)
			detail := terminated.Reason
			if terminated.Message != "" {
				detail += ": " + terminated.Message
			}
			report.Findings = append(report.Findings, Finding{Title: title, Detail: detail})
			if report.Summary == "" {
				report.Summary = summary
			}
			logContainers = append(logContainers, status.Name)
		}
	}
	inspect(pod.Status.InitContainerStatuses)
	inspect(pod.Status.ContainerStatuses)

	return report, unique(logContainers)
}

func waitingFinding(container, reason string) (string, string) {
	switch reason {
	case "ImagePullBackOff", "ErrImagePull":
		return "Image pull failed", "Image pull failed for container " + container
	case "CrashLoopBackOff":
		return "Container crash loop", "Container " + container + " is crash looping"
	default:
		return "Container waiting", "Container " + container + " is waiting: " + reason
	}
}

func terminatedFinding(container, reason string) (string, string) {
	if reason == "OOMKilled" {
		return "Container out of memory", "Container " + container + " was OOM killed"
	}
	return "Container terminated", "Container " + container + " terminated: " + reason
}

func (c *Client) warningEvents(ctx context.Context, pod *corev1.Pod) ([]Event, error) {
	selector := fields.OneTermEqualSelector("involvedObject.uid", string(pod.UID)).String()
	list, err := c.client.CoreV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{FieldSelector: selector})
	if err != nil {
		return nil, err
	}

	warnings := make([]corev1.Event, 0, len(list.Items))
	for _, event := range list.Items {
		if event.InvolvedObject.UID != pod.UID || event.Type != corev1.EventTypeWarning {
			continue
		}
		warnings = append(warnings, event)
	}
	sort.SliceStable(warnings, func(i, j int) bool {
		return eventTime(warnings[i]).After(eventTime(warnings[j]).Time)
	})
	if len(warnings) > maxEvents {
		warnings = warnings[:maxEvents]
	}

	out := make([]Event, 0, len(warnings))
	for _, event := range warnings {
		out = append(out, Event{Reason: event.Reason, Message: event.Message})
	}
	return out, nil
}

func eventTime(event corev1.Event) metav1.Time {
	if !event.EventTime.IsZero() {
		return metav1.NewTime(event.EventTime.Time)
	}
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp
	}
	return event.CreationTimestamp
}

func (c *Client) previousLogs(ctx context.Context, pod *corev1.Pod, container string) (string, error) {
	request := c.client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container:  container,
		Previous:   true,
		TailLines:  ptr(maxLogLines),
		LimitBytes: ptr(maxLogBytes),
	})
	stream, err := request.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, maxLogBytes))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func ptr[T any](value T) *T { return &value }

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
