package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/diagnosis"
	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/health"
	"github.com/anishetty/kgraph/internal/network"
	"github.com/anishetty/kgraph/internal/query"
	"github.com/anishetty/kgraph/internal/rbac"
	"github.com/anishetty/kgraph/internal/rules"
)

// ─── /api/contexts ────────────────────────────────────────────────────────────

type ContextsResponse struct {
	Current  string   `json:"current"`
	Contexts []string `json:"contexts"`
}

func (h *Handler) handleContexts(w http.ResponseWriter, r *http.Request) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if h.kubeconfig != "" {
		rules.ExplicitPath = h.kubeconfig
	}
	cfg, err := rules.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var names []string
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	sortStrings(names)
	writeJSON(w, ContextsResponse{Current: cfg.CurrentContext, Contexts: names})
}

// ─── /api/graph ─────────────────────────────────────────────────────────────

type NodeInfo struct {
	UID       string            `json:"uid"`
	Kind      string            `json:"kind"`
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Healthy   bool              `json:"healthy"`
	Reason    string            `json:"reason,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type EdgeInfo struct {
	From string `json:"from"`
	To   string `json:"to"`
	Rel  string `json:"rel"`
}

type GraphResponse struct {
	Context   string     `json:"context"`
	Namespace string     `json:"namespace"`
	Nodes     []NodeInfo `json:"nodes"`
	Edges     []EdgeInfo `json:"edges"`
	Total     int        `json:"total"`     // total nodes before any limit
	Truncated bool       `json:"truncated"` // true when maxNodes cap was applied
}

// maxGraphNodes is the default cap for /api/graph responses.
// Clients can override with ?maxNodes=N. 0 = unlimited.
const defaultMaxGraphNodes = 800

func (h *Handler) handleGraph(w http.ResponseWriter, r *http.Request) {
	ctx, ctxName, ns, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = ctx
	maxNodes := defaultMaxGraphNodes
	if v := r.URL.Query().Get("maxNodes"); v != "" {
		var n int
		if _, err2 := fmt.Sscanf(v, "%d", &n); err2 == nil && n > 0 {
			maxNodes = n
		}
	}
	g := graph.Build(*res)
	writeJSON(w, buildGraphResponse(g, ctxName, ns, maxNodes))
}

func buildGraphResponse(g *graph.Graph, ctxName, ns string, maxNodes int) GraphResponse {
	resp := GraphResponse{Context: ctxName, Namespace: ns}
	allNodes := g.Nodes()
	filtered := allNodes[:0:0] // same backing array, 0 length
	for _, n := range allNodes {
		if ns != "" && n.Namespace != ns && n.Namespace != "" {
			continue
		}
		filtered = append(filtered, n)
	}
	resp.Total = len(filtered)

	// Apply cap — prioritise top-level workloads and network resources so
	// the visible graph is still useful.
	truncated := maxNodes > 0 && len(filtered) > maxNodes
	if truncated {
		filtered = filtered[:maxNodes]
	}
	resp.Truncated = truncated

	// Build a set of visible UIDs for edge filtering
	visibleUIDs := make(map[string]bool, len(filtered))
	for _, n := range filtered {
		visibleUIDs[string(n.UID)] = true
		resp.Nodes = append(resp.Nodes, nodeToInfo(n))
	}
	for _, e := range g.Edges() {
		if visibleUIDs[string(e.From)] && visibleUIDs[string(e.To)] {
			resp.Edges = append(resp.Edges, EdgeInfo{
				From: string(e.From), To: string(e.To), Rel: string(e.Rel),
			})
		}
	}
	return resp
}

func nodeToInfo(n graph.Node) NodeInfo {
	ok, reason := health.Pod(n)
	if n.Kind != "Pod" {
		ok = true
	}
	info := NodeInfo{
		UID:       string(n.UID),
		Kind:      n.Kind,
		Namespace: n.Namespace,
		Name:      n.Name,
		Healthy:   ok,
		Reason:    reason,
		Fields:    extractNodeFields(n),
	}
	if obj, ok2 := n.Raw.(metav1.Object); ok2 {
		lbls := obj.GetLabels()
		if len(lbls) > 0 {
			info.Labels = lbls
		}
	}
	return info
}

func extractNodeFields(n graph.Node) map[string]string {
	f := map[string]string{}
	switch n.Kind {
	case "Pod":
		if pod, ok := n.Raw.(*corev1.Pod); ok && pod != nil {
			f["phase"] = string(pod.Status.Phase)
			if pod.Spec.NodeName != "" {
				f["nodeName"] = pod.Spec.NodeName
			}
			imgs := collectImages(pod.Spec.Containers)
			if imgs != "" {
				f["images"] = imgs
			}
		}
	case "Deployment":
		if d, ok := n.Raw.(*appsv1.Deployment); ok && d != nil {
			if d.Spec.Replicas != nil {
				f["replicas"] = fmt.Sprintf("%d", *d.Spec.Replicas)
			}
			f["readyReplicas"] = fmt.Sprintf("%d", d.Status.ReadyReplicas)
			f["availableReplicas"] = fmt.Sprintf("%d", d.Status.AvailableReplicas)
			f["images"] = collectImages(d.Spec.Template.Spec.Containers)
		}
	case "StatefulSet":
		if s, ok := n.Raw.(*appsv1.StatefulSet); ok && s != nil {
			if s.Spec.Replicas != nil {
				f["replicas"] = fmt.Sprintf("%d", *s.Spec.Replicas)
			}
			f["readyReplicas"] = fmt.Sprintf("%d", s.Status.ReadyReplicas)
			f["images"] = collectImages(s.Spec.Template.Spec.Containers)
		}
	case "DaemonSet":
		if d, ok := n.Raw.(*appsv1.DaemonSet); ok && d != nil {
			f["desired"] = fmt.Sprintf("%d", d.Status.DesiredNumberScheduled)
			f["ready"] = fmt.Sprintf("%d", d.Status.NumberReady)
			f["images"] = collectImages(d.Spec.Template.Spec.Containers)
		}
	case "Service":
		if svc, ok := n.Raw.(*corev1.Service); ok && svc != nil {
			f["type"] = string(svc.Spec.Type)
			f["clusterIP"] = svc.Spec.ClusterIP
		}
	case "PersistentVolumeClaim":
		if pvc, ok := n.Raw.(*corev1.PersistentVolumeClaim); ok && pvc != nil {
			f["phase"] = string(pvc.Status.Phase)
		}
	case "Ingress":
		if ing, ok := n.Raw.(*networkingv1.Ingress); ok && ing != nil {
			var hosts []string
			for _, r := range ing.Spec.Rules {
				if r.Host != "" {
					hosts = append(hosts, r.Host)
				}
			}
			f["hosts"] = strings.Join(hosts, ",")
		}
	}
	if len(f) == 0 {
		return nil
	}
	return f
}

func collectImages(cs []corev1.Container) string {
	imgs := make([]string, 0, len(cs))
	for _, c := range cs {
		imgs = append(imgs, c.Image)
	}
	return strings.Join(imgs, ", ")
}

// ─── /api/deps and /api/impact ───────────────────────────────────────────────

// BlastImpact classifies the survivability of a workload when a dependency
// is removed or modified. Only populated for impact-tree responses.
//
//	OUTAGE   — single replica; removing the dependency causes a complete outage
//	DEGRADED — 2–4 replicas; some pods lost but service may limp along
//	SAFE     — 5+ replicas or not a workload node; little or no impact
type BlastImpact = string

const (
	BlastOutage   BlastImpact = "OUTAGE"
	BlastDegraded BlastImpact = "DEGRADED"
	BlastSafe     BlastImpact = "SAFE"
)

type TreeNode struct {
	Node        NodeInfo    `json:"node"`
	Rel         string      `json:"rel,omitempty"`
	Cycle       bool        `json:"cycle,omitempty"`
	BlastImpact BlastImpact `json:"blastImpact,omitempty"` // impact trees only
	Children    []*TreeNode `json:"children"`
}

type TreeResponse struct {
	Context string    `json:"context"`
	Root    *TreeNode `json:"root"`
}

func (h *Handler) handleDeps(w http.ResponseWriter, r *http.Request) {
	h.handleTree(w, r, false)
}

func (h *Handler) handleImpact(w http.ResponseWriter, r *http.Request) {
	h.handleTree(w, r, true)
}

func (h *Handler) handleTree(w http.ResponseWriter, r *http.Request, impact bool) {
	ctx, ctxName, _, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = ctx
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		writeError(w, http.StatusBadRequest, "resource parameter required (e.g. deployment/frontend)")
		return
	}
	g := graph.Build(*res)
	node, findErr := findNode(g, resource)
	if findErr != nil {
		writeError(w, http.StatusNotFound, findErr.Error())
		return
	}
	var qtree *query.TreeNode
	if impact {
		qtree = query.ImpactTree(g, *node)
	} else {
		qtree = query.DependencyTree(g, *node)
	}
	writeJSON(w, TreeResponse{Context: ctxName, Root: convertTree(qtree, impact)})
}

func convertTree(qt *query.TreeNode, annotateBlast bool) *TreeNode {
	if qt == nil {
		return nil
	}
	t := &TreeNode{
		Node:  nodeToInfo(qt.Node),
		Rel:   string(qt.Rel),
		Cycle: qt.Cycle,
	}
	if annotateBlast {
		t.BlastImpact = computeBlastImpact(qt.Node)
	}
	for _, c := range qt.Children {
		t.Children = append(t.Children, convertTree(c, annotateBlast))
	}
	return t
}

// computeBlastImpact returns the blast-radius severity for a workload node
// based on its desired replica count.
func computeBlastImpact(n graph.Node) BlastImpact {
	switch obj := n.Raw.(type) {
	case *appsv1.Deployment:
		if obj == nil {
			return ""
		}
		r := int32(1)
		if obj.Spec.Replicas != nil {
			r = *obj.Spec.Replicas
		}
		return replicasToBlast(r)
	case *appsv1.StatefulSet:
		if obj == nil {
			return ""
		}
		r := int32(1)
		if obj.Spec.Replicas != nil {
			r = *obj.Spec.Replicas
		}
		return replicasToBlast(r)
	case *appsv1.DaemonSet:
		if obj == nil {
			return ""
		}
		return replicasToBlast(obj.Status.DesiredNumberScheduled)
	default:
		return ""
	}
}

func replicasToBlast(replicas int32) BlastImpact {
	switch {
	case replicas <= 1:
		return BlastOutage
	case replicas < 5:
		return BlastDegraded
	default:
		return BlastSafe
	}
}

// ─── /api/why ────────────────────────────────────────────────────────────────

type PodDiagnosis struct {
	Pod      NodeInfo            `json:"pod"`
	Status   string              `json:"status"`
	Summary  string              `json:"summary,omitempty"`
	Findings []diagnosis.Finding `json:"findings,omitempty"`
	Events   []diagnosis.Event   `json:"events,omitempty"`
	Logs     []diagnosis.Log     `json:"logs,omitempty"`
}

type WhyResponse struct {
	Context   string         `json:"context"`
	Node      NodeInfo       `json:"node"`
	Tree      *TreeNode      `json:"tree"`
	Diagnoses []PodDiagnosis `json:"diagnoses"`
}

func (h *Handler) handleWhy(w http.ResponseWriter, r *http.Request) {
	reqCtx, ctxName, _, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		writeError(w, http.StatusBadRequest, "resource parameter required")
		return
	}
	g := graph.Build(*res)
	node, findErr := findNode(g, resource)
	if findErr != nil {
		writeError(w, http.StatusNotFound, findErr.Error())
		return
	}

	qtree := query.DependencyTree(g, *node)
	resp := WhyResponse{
		Context: ctxName,
		Node:    nodeToInfo(*node),
		Tree:    convertTree(qtree, false),
	}

	// Direct diagnosis for Service / PVC.
	directKinds := map[string]bool{"PersistentVolumeClaim": true, "Service": true}
	diagClient, diagErr := diagnosis.New(h.kubeconfig, ctxName)
	if directKinds[node.Kind] && diagErr == nil {
		dctx, cancel := context.WithTimeout(reqCtx, 10*time.Second)
		rep, _ := diagClient.Diagnose(dctx, *node)
		cancel()
		if !rep.Empty() {
			_, reason := health.Pod(*node)
			resp.Diagnoses = append(resp.Diagnoses, PodDiagnosis{
				Pod:      nodeToInfo(*node),
				Status:   reason,
				Summary:  rep.Summary,
				Findings: rep.Findings,
				Events:   rep.Events,
				Logs:     rep.Logs,
			})
		}
	}

	// Unhealthy pods in the dep tree.
	unhealthy := collectUnhealthyNodes(qtree)
	for _, podNode := range unhealthy {
		_, reason := health.Pod(podNode)
		pd := PodDiagnosis{Pod: nodeToInfo(podNode), Status: reason}
		if diagErr == nil {
			dctx, cancel := context.WithTimeout(reqCtx, 10*time.Second)
			rep, _ := diagClient.Diagnose(dctx, podNode)
			cancel()
			pd.Summary = rep.Summary
			pd.Findings = rep.Findings
			pd.Events = rep.Events
			pd.Logs = rep.Logs
		}
		resp.Diagnoses = append(resp.Diagnoses, pd)
	}

	writeJSON(w, resp)
}

func collectUnhealthyNodes(root *query.TreeNode) []graph.Node {
	seen := map[string]bool{}
	var out []graph.Node
	var walk func(n *query.TreeNode)
	walk = func(n *query.TreeNode) {
		if n.Node.Kind == "Pod" && !n.Cycle {
			if ok, _ := health.Pod(n.Node); !ok {
				if !seen[string(n.Node.UID)] {
					seen[string(n.Node.UID)] = true
					out = append(out, n.Node)
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// ─── /api/doctor ─────────────────────────────────────────────────────────────

type DoctorResponse struct {
	Context  string        `json:"context"`
	Findings []FindingInfo `json:"findings"`
}

type FindingInfo struct {
	Severity  string `json:"severity"`
	Category  string `json:"category"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Issue     string `json:"issue"`
	Detail    string `json:"detail"`
	Fix       string `json:"fix"`
}

func (h *Handler) handleDoctor(w http.ResponseWriter, r *http.Request) {
	_, ctxName, ns, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	g := graph.Build(*res)
	findings := rules.Scan(g, ns)
	resp := DoctorResponse{Context: ctxName, Findings: []FindingInfo{}}
	for _, f := range findings {
		sev := "warning"
		if f.Severity == rules.SeverityBlocking {
			sev = "blocking"
		}
		resp.Findings = append(resp.Findings, FindingInfo{
			Severity: sev, Category: f.Category,
			Kind: f.Kind, Namespace: f.Namespace, Name: f.Name,
			Issue: f.Issue, Detail: f.Detail, Fix: f.Fix,
		})
	}
	writeJSON(w, resp)
}

// ─── /api/orphan ─────────────────────────────────────────────────────────────

type OrphanResponse struct {
	Context string       `json:"context"`
	Orphans []OrphanInfo `json:"orphans"`
}

type OrphanInfo struct {
	Node   NodeInfo `json:"node"`
	Reason string   `json:"reason"`
}

func (h *Handler) handleOrphan(w http.ResponseWriter, r *http.Request) {
	_, ctxName, ns, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	g := graph.Build(*res)
	orphans := query.Orphaned(g, ns)
	resp := OrphanResponse{Context: ctxName, Orphans: []OrphanInfo{}}
	for _, o := range orphans {
		resp.Orphans = append(resp.Orphans, OrphanInfo{
			Node: nodeToInfo(o.Node), Reason: o.Reason,
		})
	}
	writeJSON(w, resp)
}

// ─── /api/events ─────────────────────────────────────────────────────────────

type EventInfo struct {
	Age     string `json:"age"`
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Object  string `json:"object"`
	Message string `json:"message"`
}

type EventsResponse struct {
	Context string      `json:"context"`
	Events  []EventInfo `json:"events"`
}

func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	reqCtx, ctxName, ns, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		writeError(w, http.StatusBadRequest, "resource parameter required")
		return
	}
	g := graph.Build(*res)
	node, findErr := findNode(g, resource)
	if findErr != nil {
		writeError(w, http.StatusNotFound, findErr.Error())
		return
	}

	restConfig, err := collector.RESTConfig(h.kubeconfig, ctxName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Collect target names: the node + its pod descendants.
	type target struct{ kind, namespace, name string }
	targets := []target{{node.Kind, node.Namespace, node.Name}}
	seen := map[string]bool{string(node.UID): true}
	queue := g.Out(node.UID)
	for len(queue) > 0 {
		uid := queue[0]
		queue = queue[1:]
		if seen[string(uid)] {
			continue
		}
		seen[string(uid)] = true
		n, ok := g.Node(uid)
		if ok && n.Kind == "Pod" {
			targets = append(targets, target{n.Kind, n.Namespace, n.Name})
		}
		queue = append(queue, g.Out(uid)...)
	}

	if ns == "" {
		ns = node.Namespace
	}

	fetchCtx, cancel := context.WithTimeout(reqCtx, 15*time.Second)
	defer cancel()

	resp := EventsResponse{Context: ctxName, Events: []EventInfo{}}
	now := time.Now()
	for _, t := range targets {
		// Only filter by name — the List call is already scoped to the namespace,
		// so adding involvedObject.namespace to the selector is redundant and can
		// silently fail on some Kubernetes versions.
		sel := "involvedObject.name=" + t.name
		list, listErr := client.CoreV1().Events(t.namespace).List(fetchCtx, metav1.ListOptions{FieldSelector: sel})
		if listErr != nil {
			continue
		}
		for _, ev := range list.Items {
			ts := ev.LastTimestamp.Time
			if ts.IsZero() {
				ts = ev.EventTime.Time
			}
			if ts.IsZero() {
				ts = ev.CreationTimestamp.Time
			}
			resp.Events = append(resp.Events, EventInfo{
				Age:     formatAge(now.Sub(ts)),
				Type:    ev.Type,
				Reason:  ev.Reason,
				Object:  t.kind + "/" + t.name,
				Message: ev.Message,
			})
		}
	}
	// Newest first
	for i, j := 0, len(resp.Events)-1; i < j; i, j = i+1, j-1 {
		resp.Events[i], resp.Events[j] = resp.Events[j], resp.Events[i]
	}
	writeJSON(w, resp)
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// ─── /api/rbac ───────────────────────────────────────────────────────────────

type RBACResponse struct {
	Context        string           `json:"context"`
	ServiceAccount string           `json:"serviceAccount"`
	Namespace      string           `json:"namespace"`
	Verb           string           `json:"verb"`
	Resource       string           `json:"resource"`
	APIGroup       string           `json:"apiGroup"`
	Verdict        string           `json:"verdict"`
	Paths          []rbac.GrantPath `json:"paths,omitempty"`
	Checks         []RBACCheckInfo  `json:"checks"`
}

type RBACCheckInfo struct {
	BindingKind  string `json:"bindingKind"`
	BindingName  string `json:"bindingName"`
	BindingNS    string `json:"bindingNs"`
	RoleKind     string `json:"roleKind"`
	RoleName     string `json:"roleName"`
	SubjectMatch bool   `json:"subjectMatch"`
	Grants       bool   `json:"grants"`
}

func (h *Handler) handleRBAC(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctxName := q.Get("context")
	ns := q.Get("namespace")
	if ns == "" {
		ns = "default"
	}
	saName := q.Get("sa")
	verb := q.Get("verb")
	resource := q.Get("resource")
	apiGroup := q.Get("apiGroup")

	if saName == "" || verb == "" || resource == "" {
		writeError(w, http.StatusBadRequest, "sa, verb, and resource parameters required")
		return
	}

	_, _, _, res, err := h.collectWithCtx(r.Context(), ctxName, ns)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var sa *corev1.ServiceAccount
	for i := range res.ServiceAccounts {
		s := &res.ServiceAccounts[i]
		if s.Name == saName && s.Namespace == ns {
			sa = s
			break
		}
	}
	if sa == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("ServiceAccount %q not found in namespace %q", saName, ns))
		return
	}

	result := rbac.Check(*sa, verb, resource, apiGroup, ns,
		res.Roles, res.ClusterRoles, res.RoleBindings, res.ClusterRoleBindings)

	resp := RBACResponse{
		Context:        ctxName,
		ServiceAccount: saName,
		Namespace:      ns,
		Verb:           verb,
		Resource:       resource,
		APIGroup:       apiGroup,
		Verdict:        result.Verdict.String(),
		Paths:          result.Paths,
	}
	for _, c := range result.Checks {
		resp.Checks = append(resp.Checks, RBACCheckInfo{
			BindingKind:  c.BindingKind,
			BindingName:  c.BindingName,
			BindingNS:    c.BindingNS,
			RoleKind:     c.RoleKind,
			RoleName:     c.RoleName,
			SubjectMatch: c.SubjectMatch,
			Grants:       c.Grants,
		})
	}
	writeJSON(w, resp)
}

// ─── /api/network ─────────────────────────────────────────────────────────────

type NetworkHop struct {
	Kind      string   `json:"kind"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Healthy   bool     `json:"healthy"`
	Ready     int      `json:"ready"`
	Total     int      `json:"total"`
	Selector  string   `json:"selector,omitempty"`
	Pods      []PodHop `json:"pods,omitempty"`
}

type PodHop struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Healthy   bool   `json:"healthy"`
	Reason    string `json:"reason,omitempty"`
}

type NetworkResponse struct {
	Context  string       `json:"context"`
	Resource string       `json:"resource"`
	Hops     []NetworkHop `json:"hops"`
}

func (h *Handler) handleNetwork(w http.ResponseWriter, r *http.Request) {
	_, ctxName, _, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		writeError(w, http.StatusBadRequest, "resource parameter required")
		return
	}
	g := graph.Build(*res)
	node, findErr := findNode(g, resource)
	if findErr != nil {
		writeError(w, http.StatusNotFound, findErr.Error())
		return
	}

	resp := NetworkResponse{Context: ctxName, Resource: resource}

	var processService func(uid string)
	processService = func(uid string) {
		svcNode, ok := g.Node(k8stypes.UID(uid))
		if !ok || svcNode.Kind != "Service" {
			return
		}
		svc, _ := svcNode.Raw.(*corev1.Service)
		hop := NetworkHop{Kind: "Service", Namespace: svcNode.Namespace, Name: svcNode.Name}
		if svc != nil {
			var selParts []string
			for k, v := range svc.Spec.Selector {
				selParts = append(selParts, k+"="+v)
			}
			hop.Selector = strings.Join(selParts, ", ")
		}
		for _, podUID := range g.Out(svcNode.UID) {
			podNode, ok := g.Node(podUID)
			if !ok || podNode.Kind != "Pod" {
				continue
			}
			isHealthy, reason := health.Pod(podNode)
			hop.Pods = append(hop.Pods, PodHop{
				Name: podNode.Name, Namespace: podNode.Namespace,
				Healthy: isHealthy, Reason: reason,
			})
			hop.Total++
			if isHealthy {
				hop.Ready++
			}
		}
		hop.Healthy = hop.Ready == hop.Total && hop.Total > 0
		resp.Hops = append(resp.Hops, hop)
	}

	if node.Kind == "Service" {
		processService(string(node.UID))
	} else if node.Kind == "Ingress" {
		if ing, ok := node.Raw.(*networkingv1.Ingress); ok && ing != nil {
			for _, e := range g.OutEdges(node.UID) {
				if e.Rel == graph.RelRoutes {
					processService(string(e.To))
				}
			}
		}
	}

	writeJSON(w, resp)
}

// ─── /api/can-reach ──────────────────────────────────────────────────────────

type CanReachResponse struct {
	Context  string        `json:"context"`
	Source   string        `json:"source"`
	Dest     string        `json:"dest"`
	Port     int32         `json:"port"`
	Protocol string        `json:"protocol"`
	Results  []ReachResult `json:"results"`
}

type ReachResult struct {
	DstPod      string `json:"dstPod"`
	Verdict     string `json:"verdict"`
	EgressOpen  bool   `json:"egressOpen"`
	IngressOpen bool   `json:"ingressOpen"`
	Suggestion  string `json:"suggestion,omitempty"`
}

func (h *Handler) handleCanReach(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctxName := q.Get("context")
	ns := q.Get("namespace")
	if ns == "" {
		ns = "default"
	}
	srcRef := q.Get("src")
	dstRef := q.Get("dst")
	portStr := q.Get("port")
	proto := corev1.Protocol(strings.ToUpper(q.Get("protocol")))
	if proto == "" {
		proto = corev1.ProtocolTCP
	}
	var port int32
	fmt.Sscanf(portStr, "%d", &port)

	_, _, _, res, err := h.collectWithCtx(r.Context(), ctxName, ns)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	g := graph.Build(*res)
	srcNode, findErr := findNode(g, srcRef)
	if findErr != nil || srcNode.Kind != "Pod" {
		writeError(w, http.StatusBadRequest, "src must be a pod reference")
		return
	}
	srcPod, _ := srcNode.Raw.(*corev1.Pod)
	if srcPod == nil {
		writeError(w, http.StatusBadRequest, "could not read source pod")
		return
	}

	// Resolve destination pods.
	var dstPods []corev1.Pod
	dstNode, findErr2 := findNode(g, dstRef)
	if findErr2 == nil {
		if dstNode.Kind == "Pod" {
			if p, ok := dstNode.Raw.(*corev1.Pod); ok {
				dstPods = append(dstPods, *p)
			}
		} else if dstNode.Kind == "Service" {
			for _, uid := range g.Out(dstNode.UID) {
				n, ok := g.Node(uid)
				if ok && n.Kind == "Pod" {
					if p, ok := n.Raw.(*corev1.Pod); ok {
						dstPods = append(dstPods, *p)
					}
				}
			}
		}
	}
	if len(dstPods) == 0 {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no pods found for %q", dstRef))
		return
	}

	nsLabels := network.NamespaceLabelsFromList(res.Namespaces)
	policies := network.SortPolicies(res.NetworkPolicies)

	resp := CanReachResponse{
		Context: ctxName, Source: srcRef, Dest: dstRef,
		Port: port, Protocol: string(proto),
	}
	for _, dst := range dstPods {
		result := network.Check(*srcPod, dst, port, proto, policies, nsLabels)
		resp.Results = append(resp.Results, ReachResult{
			DstPod:      dst.Namespace + "/" + dst.Name,
			Verdict:     result.Verdict.String(),
			EgressOpen:  result.Egress.Open,
			IngressOpen: result.Ingress.Open,
			Suggestion:  network.Suggestion(result),
		})
	}
	writeJSON(w, resp)
}

// ─── shared helpers ──────────────────────────────────────────────────────────

// collect parses query params and runs the collector.
func (h *Handler) collect(r *http.Request) (context.Context, string, string, *graph.Resources, error) {
	q := r.URL.Query()
	ctxName := q.Get("context")
	ns := q.Get("namespace")
	refresh := q.Get("refresh") == "true"
	return h.collectOpts(r.Context(), ctxName, ns, refresh)
}

func (h *Handler) collectWithCtx(ctx context.Context, ctxName, ns string) (context.Context, string, string, *graph.Resources, error) {
	return h.collectOpts(ctx, ctxName, ns, false)
}

// collectOpts resolves the kubeconfig context, checks the shared resource cache,
// and only sweeps the cluster when the cache is cold or refresh is forced.
func (h *Handler) collectOpts(ctx context.Context, ctxName, ns string, refresh bool) (context.Context, string, string, *graph.Resources, error) {
	if ctxName == "" {
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		if h.kubeconfig != "" {
			rules.ExplicitPath = h.kubeconfig
		}
		cfg, err := rules.Load()
		if err != nil {
			return ctx, "", ns, nil, err
		}
		ctxName = cfg.CurrentContext
	}

	// Cache hit — skip the 21-API sweep.
	if !refresh {
		if cached, ok := h.cache.get(ctxName); ok {
			return ctx, ctxName, ns, cached, nil
		}
	} else {
		h.cache.invalidate(ctxName)
	}

	c, err := collector.New(h.kubeconfig, ctxName)
	if err != nil {
		return ctx, ctxName, ns, nil, err
	}
	collectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res, err := c.Collect(collectCtx)
	if err != nil {
		return ctx, ctxName, ns, nil, err
	}
	h.cache.set(ctxName, res)
	return ctx, ctxName, ns, &res, nil
}

func findNode(g *graph.Graph, ref string) (*graph.Node, error) {
	kind, name := parseResourceRef(ref)
	for _, n := range g.Nodes() {
		// Exact match preferred; fall back to suffix match for generated names
		// (e.g. "frontend-abc-xyz" matches "frontend-abc-xyz").
		if strings.EqualFold(n.Kind, kind) && n.Name == name {
			n := n
			return &n, nil
		}
	}
	// Second pass: prefix/substring match for truncated names
	for _, n := range g.Nodes() {
		if strings.EqualFold(n.Kind, kind) && strings.Contains(n.Name, name) {
			n := n
			return &n, nil
		}
	}
	return nil, fmt.Errorf("%q not found in graph", ref)
}

func parseResourceRef(ref string) (kind, name string) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		return expandKind(parts[0]), parts[1]
	}
	return "", ref
}

var kindAliasMap = map[string]string{
	"deploy": "Deployment", "deployment": "Deployment", "deployments": "Deployment",
	"pod": "Pod", "pods": "Pod", "po": "Pod",
	"svc": "Service", "service": "Service", "services": "Service",
	"rs": "ReplicaSet", "replicaset": "ReplicaSet",
	"sts": "StatefulSet", "statefulset": "StatefulSet",
	"ds": "DaemonSet", "daemonset": "DaemonSet",
	"job": "Job", "jobs": "Job",
	"cj": "CronJob", "cronjob": "CronJob",
	"ing": "Ingress", "ingress": "Ingress",
	"cm": "ConfigMap", "configmap": "ConfigMap",
	"secret": "Secret", "secrets": "Secret",
	"pvc": "PersistentVolumeClaim",
	"pv":  "PersistentVolume",
	"sa":  "ServiceAccount", "serviceaccount": "ServiceAccount",
	"no": "Node", "node": "Node", "nodes": "Node",
}

func expandKind(alias string) string {
	lower := strings.ToLower(alias)
	if k, ok := kindAliasMap[lower]; ok {
		return k
	}
	return alias
}

func sortStrings(s []string) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
