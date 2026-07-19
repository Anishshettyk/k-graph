package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/graph"
)

// resourceRef is a parsed <kind>/<name> command argument.
type resourceRef struct {
	Kind string // canonical kind, e.g. "Deployment"
	Name string
}

// kindAliases maps common shorthands to canonical kinds supported by the
// collector.
var kindAliases = map[string]string{
	"deploy":                "Deployment",
	"deployment":            "Deployment",
	"deployments":           "Deployment",
	"rs":                    "ReplicaSet",
	"replicaset":            "ReplicaSet",
	"replicasets":           "ReplicaSet",
	"sts":                   "StatefulSet",
	"statefulset":           "StatefulSet",
	"statefulsets":          "StatefulSet",
	"ds":                    "DaemonSet",
	"daemonset":             "DaemonSet",
	"daemonsets":            "DaemonSet",
	"job":                   "Job",
	"jobs":                  "Job",
	"cronjob":               "CronJob",
	"cronjobs":              "CronJob",
	"cj":                    "CronJob",
	"po":                    "Pod",
	"pod":                   "Pod",
	"pods":                  "Pod",
	"svc":                   "Service",
	"service":               "Service",
	"services":              "Service",
	"ing":                   "Ingress",
	"ingress":               "Ingress",
	"ingresses":             "Ingress",
	"cm":                    "ConfigMap",
	"configmap":             "ConfigMap",
	"configmaps":            "ConfigMap",
	"secret":                "Secret",
	"secrets":               "Secret",
	"pvc":                   "PersistentVolumeClaim",
	"persistentvolumeclaim": "PersistentVolumeClaim",
	"pv":                    "PersistentVolume",
	"persistentvolume":      "PersistentVolume",
	"node":                  "Node",
	"nodes":                 "Node",
	"no":                    "Node",
	"sa":                    "ServiceAccount",
	"serviceaccount":        "ServiceAccount",
	"serviceaccounts":       "ServiceAccount",
}

// parseRef splits a "<kind>/<name>" argument and normalizes the kind.
func parseRef(arg string) (resourceRef, error) {
	kind, name, ok := strings.Cut(arg, "/")
	if !ok || kind == "" || name == "" {
		return resourceRef{}, fmt.Errorf("invalid resource %q: expected <kind>/<name>, e.g. deployment/frontend", arg)
	}
	canonical, ok := kindAliases[strings.ToLower(kind)]
	if !ok {
		return resourceRef{}, fmt.Errorf("unsupported kind %q: try deployment, statefulset, daemonset, job, cronjob, pod, service, ingress, configmap, secret, pvc, pv, node, or serviceaccount", kind)
	}
	return resourceRef{Kind: canonical, Name: name}, nil
}

// buildGraph connects to the cluster (read-only), collects resources, and
// builds the knowledge graph.
func buildGraph(ctx context.Context) (*graph.Graph, error) {
	c, err := collector.New(kubeconfig, kubeContext)
	if err != nil {
		return nil, err
	}
	res, err := c.Collect(ctx)
	if err != nil {
		return nil, err
	}
	return graph.Build(res), nil
}

// buildResources collects raw cluster resources and returns both the resolved
// context name and the full resource snapshot. Used by commands that need the
// raw objects beyond the graph (e.g. doctor, rbac).
func buildResources(ctx context.Context) (ctxName string, res graph.Resources, err error) {
	ctxName = kubeContext
	if ctxName == "" {
		cfg, loadErr := loadKubeconfig()
		if loadErr != nil {
			return "", res, fmt.Errorf("load kubeconfig: %w", loadErr)
		}
		ctxName = cfg.CurrentContext
	}
	c, newErr := collector.New(kubeconfig, ctxName)
	if newErr != nil {
		return "", res, newErr
	}
	res, err = c.Collect(ctx)
	return
}

// buildLiveGraph resolves the current context name, collects the cluster, and
// returns both the context name and the built graph. Used by commands that need
// both the context name and the graph (e.g. check, orphan, can-reach).
func buildLiveGraph(ctx context.Context) (string, *graph.Graph, error) {
	ctxName, res, err := buildResources(ctx)
	if err != nil {
		return "", nil, err
	}
	return ctxName, graph.Build(res), nil
}

// resolveNode finds exactly one node matching the reference, returning a
// helpful error if there are zero or multiple matches.
func resolveNode(g *graph.Graph, ref resourceRef) (graph.Node, error) {
	matches := g.Find(ref.Kind, ref.Name, "")
	switch len(matches) {
	case 0:
		return graph.Node{}, fmt.Errorf("%s/%s not found in cluster", strings.ToLower(ref.Kind), ref.Name)
	case 1:
		return matches[0], nil
	default:
		var namespaces []string
		for _, m := range matches {
			namespaces = append(namespaces, m.Namespace)
		}
		return graph.Node{}, fmt.Errorf("%s/%s is ambiguous across namespaces %s; disambiguation by namespace is not yet supported",
			strings.ToLower(ref.Kind), ref.Name, strings.Join(namespaces, ", "))
	}
}

// describe formats a node as "Kind namespace/name".
func describe(n graph.Node) string {
	if n.Namespace == "" {
		return fmt.Sprintf("%s %s", n.Kind, n.Name)
	}
	return fmt.Sprintf("%s %s/%s", n.Kind, n.Namespace, n.Name)
}

// colorEnabled reports whether ANSI styling should be used for w. It honors the
// NO_COLOR convention and only enables color when writing to a terminal.
func colorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// completionKinds are the canonical kinds offered before a "/" is typed.
var completionKinds = []string{
	"deployment", "statefulset", "daemonset", "job", "cronjob", "pod",
	"service", "ingress", "configmap", "secret", "persistentvolumeclaim",
	"persistentvolume", "node", "serviceaccount",
}

// completeResourceRef provides shell completion for a <kind>/<name> argument:
// it suggests kinds until "/" is typed, then live resource names of that kind
// from the cluster.
func completeResourceRef(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	kindPart, namePart, hasSlash := strings.Cut(toComplete, "/")
	if !hasSlash {
		var out []string
		for _, k := range completionKinds {
			if strings.HasPrefix(k, strings.ToLower(toComplete)) {
				out = append(out, k+"/")
			}
		}
		return out, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
	}

	canonical, ok := kindAliases[strings.ToLower(kindPart)]
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	g, err := buildGraph(cmd.Context())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, n := range g.Nodes() {
		if n.Kind == canonical && strings.HasPrefix(n.Name, namePart) {
			out = append(out, kindPart+"/"+n.Name)
		}
	}
	sort.Strings(out)
	return out, cobra.ShellCompDirectiveNoFileComp
}
