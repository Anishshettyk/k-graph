// Package collector connects to a cluster via client-go and reads resources
// into a graph.Resources snapshot. It is strictly read-only: it only lists
// (and later watches) objects, never mutating cluster state.
package collector

import (
	"context"
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/anishetty/kgraph/internal/graph"
)

// Collector reads cluster resources using a Kubernetes clientset.
type Collector struct {
	client kubernetes.Interface
}

// New builds a Collector from a kubeconfig path and optional context name.
// If contextName is empty, the kubeconfig's current-context is used.
func New(kubeconfig, contextName string) (*Collector, error) {
	restConfig, err := RESTConfig(kubeconfig, contextName)
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	return &Collector{client: client}, nil
}

// RESTConfig builds a *rest.Config from a kubeconfig path and optional context
// name. Exposed so other read-only clients (e.g. the metrics client) can share
// the same connection settings.
func RESTConfig(kubeconfig, contextName string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}

	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}
	return restConfig, nil
}

// NewWithClient builds a Collector from an existing clientset. Useful for tests
// with a fake client.
func NewWithClient(client kubernetes.Interface) *Collector {
	return &Collector{client: client}
}

// Collect lists the supported resources across all namespaces and returns them
// as a graph.Resources snapshot. Listing is best-effort: if a resource type
// cannot be listed (e.g. RBAC denies Secrets), that type is skipped rather than
// failing the whole collection. An error is only returned if nothing could be
// Collect fetches all supported resource types in parallel and returns them as a
// typed snapshot. Best-effort: if individual list calls fail (RBAC, not installed,
// etc.) those fields are left empty and the error is only returned when ALL calls
// fail.
func (c *Collector) Collect(ctx context.Context) (graph.Resources, error) {
	var (
		res      graph.Resources
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
		ok       int
	)

	opts := metav1.ListOptions{}
	all := metav1.NamespaceAll

	// Each call writes to a distinct field of res so no field-level mutex needed;
	// only the error/ok counters require synchronisation.
	try := func(name string, list func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := list(); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("list %s: %w", name, err)
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			ok++
			mu.Unlock()
		}()
	}

	try("deployments", func() error {
		l, err := c.client.AppsV1().Deployments(all).List(ctx, opts)
		if err == nil {
			res.Deployments = l.Items
		}
		return err
	})
	try("replicasets", func() error {
		l, err := c.client.AppsV1().ReplicaSets(all).List(ctx, opts)
		if err == nil {
			res.ReplicaSets = l.Items
		}
		return err
	})
	try("statefulsets", func() error {
		l, err := c.client.AppsV1().StatefulSets(all).List(ctx, opts)
		if err == nil {
			res.StatefulSets = l.Items
		}
		return err
	})
	try("daemonsets", func() error {
		l, err := c.client.AppsV1().DaemonSets(all).List(ctx, opts)
		if err == nil {
			res.DaemonSets = l.Items
		}
		return err
	})
	try("jobs", func() error {
		l, err := c.client.BatchV1().Jobs(all).List(ctx, opts)
		if err == nil {
			res.Jobs = l.Items
		}
		return err
	})
	try("cronjobs", func() error {
		l, err := c.client.BatchV1().CronJobs(all).List(ctx, opts)
		if err == nil {
			res.CronJobs = l.Items
		}
		return err
	})
	try("pods", func() error {
		l, err := c.client.CoreV1().Pods(all).List(ctx, opts)
		if err == nil {
			res.Pods = l.Items
		}
		return err
	})
	try("services", func() error {
		l, err := c.client.CoreV1().Services(all).List(ctx, opts)
		if err == nil {
			res.Services = l.Items
		}
		return err
	})
	try("ingresses", func() error {
		l, err := c.client.NetworkingV1().Ingresses(all).List(ctx, opts)
		if err == nil {
			res.Ingresses = l.Items
		}
		return err
	})
	try("configmaps", func() error {
		l, err := c.client.CoreV1().ConfigMaps(all).List(ctx, opts)
		if err == nil {
			res.ConfigMaps = l.Items
		}
		return err
	})
	try("secrets", func() error {
		l, err := c.client.CoreV1().Secrets(all).List(ctx, opts)
		if err == nil {
			res.Secrets = l.Items
		}
		return err
	})
	try("persistentvolumeclaims", func() error {
		l, err := c.client.CoreV1().PersistentVolumeClaims(all).List(ctx, opts)
		if err == nil {
			res.PVCs = l.Items
		}
		return err
	})
	try("persistentvolumes", func() error {
		l, err := c.client.CoreV1().PersistentVolumes().List(ctx, opts)
		if err == nil {
			res.PVs = l.Items
		}
		return err
	})
	try("nodes", func() error {
		l, err := c.client.CoreV1().Nodes().List(ctx, opts)
		if err == nil {
			res.Nodes = l.Items
		}
		return err
	})
	try("serviceaccounts", func() error {
		l, err := c.client.CoreV1().ServiceAccounts(all).List(ctx, opts)
		if err == nil {
			res.ServiceAccounts = l.Items
		}
		return err
	})
	try("networkpolicies", func() error {
		l, err := c.client.NetworkingV1().NetworkPolicies(all).List(ctx, opts)
		if err == nil {
			res.NetworkPolicies = l.Items
		}
		return err
	})
	try("namespaces", func() error {
		l, err := c.client.CoreV1().Namespaces().List(ctx, opts)
		if err == nil {
			res.Namespaces = l.Items
		}
		return err
	})
	try("roles", func() error {
		l, err := c.client.RbacV1().Roles(all).List(ctx, opts)
		if err == nil {
			res.Roles = l.Items
		}
		return err
	})
	try("clusterroles", func() error {
		l, err := c.client.RbacV1().ClusterRoles().List(ctx, opts)
		if err == nil {
			res.ClusterRoles = l.Items
		}
		return err
	})
	try("rolebindings", func() error {
		l, err := c.client.RbacV1().RoleBindings(all).List(ctx, opts)
		if err == nil {
			res.RoleBindings = l.Items
		}
		return err
	})
	try("clusterrolebindings", func() error {
		l, err := c.client.RbacV1().ClusterRoleBindings().List(ctx, opts)
		if err == nil {
			res.ClusterRoleBindings = l.Items
		}
		return err
	})

	wg.Wait()

	if ok == 0 && firstErr != nil {
		return res, firstErr
	}
	return res, nil
}
