// Package metrics reads live CPU/memory usage from the Kubernetes metrics API
// (metrics.k8s.io, the same source as `kubectl top`) and combines it with the
// requests/limits declared on pod specs to surface utilization and bottlenecks.
// It is strictly read-only.
package metrics

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/anishetty/kgraph/internal/collector"
)

// Client reads pod and node metrics.
type Client struct {
	mc metricsv.Interface
}

// New builds a metrics Client from a kubeconfig path and optional context.
func New(kubeconfig, contextName string) (*Client, error) {
	cfg, err := collector.RESTConfig(kubeconfig, contextName)
	if err != nil {
		return nil, err
	}
	mc, err := metricsv.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create metrics client: %w", err)
	}
	return &Client{mc: mc}, nil
}

// PodUsage is the summed live CPU/memory usage of a pod's containers.
type PodUsage struct {
	Namespace string
	Name      string
	CPUMilli  int64 // millicores
	MemBytes  int64
}

// Pods lists live usage for all pods across namespaces. A metrics-server (or
// equivalent) must be installed; otherwise the API returns an error.
func (c *Client) Pods(ctx context.Context) ([]PodUsage, error) {
	list, err := c.mc.MetricsV1beta1().PodMetricses(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pod metrics: %w", err)
	}
	out := make([]PodUsage, 0, len(list.Items))
	for i := range list.Items {
		pm := &list.Items[i]
		var cpu, mem int64
		for _, cont := range pm.Containers {
			cpu += cont.Usage.Cpu().MilliValue()
			mem += cont.Usage.Memory().Value()
		}
		out = append(out, PodUsage{Namespace: pm.Namespace, Name: pm.Name, CPUMilli: cpu, MemBytes: mem})
	}
	return out, nil
}

// NodeUsage is the live CPU/memory usage of a node.
type NodeUsage struct {
	Name     string
	CPUMilli int64
	MemBytes int64
}

// Nodes lists live usage for all nodes.
func (c *Client) Nodes(ctx context.Context) ([]NodeUsage, error) {
	list, err := c.mc.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list node metrics: %w", err)
	}
	out := make([]NodeUsage, 0, len(list.Items))
	for i := range list.Items {
		nm := &list.Items[i]
		out = append(out, NodeUsage{
			Name:     nm.Name,
			CPUMilli: nm.Usage.Cpu().MilliValue(),
			MemBytes: nm.Usage.Memory().Value(),
		})
	}
	return out, nil
}

// PodResources sums the CPU/memory requests and limits declared across a pod's
// regular containers. A zero limit means no limit was set.
type PodResources struct {
	CPUReqMilli   int64
	CPULimitMilli int64
	MemReqBytes   int64
	MemLimitBytes int64
}

// ResourcesOf sums requests and limits from a pod spec.
func ResourcesOf(pod *corev1.Pod) PodResources {
	var r PodResources
	for i := range pod.Spec.Containers {
		req := pod.Spec.Containers[i].Resources.Requests
		lim := pod.Spec.Containers[i].Resources.Limits
		r.CPUReqMilli += req.Cpu().MilliValue()
		r.MemReqBytes += req.Memory().Value()
		r.CPULimitMilli += lim.Cpu().MilliValue()
		r.MemLimitBytes += lim.Memory().Value()
	}
	return r
}
