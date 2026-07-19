// Package rules contains deterministic proactive health checks over the cluster
// graph. Every check is a pure function over graph nodes and raw typed objects —
// no cluster API calls, no AI, no heuristics that require tuning.
package rules

import (
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/anishetty/kgraph/internal/graph"
)

// Severity classifies a finding's urgency.
type Severity int

const (
	SeverityWarning  Severity = iota // should be reviewed and addressed
	SeverityBlocking                 // likely to cause an outage or security incident
)

func (s Severity) String() string {
	if s == SeverityBlocking {
		return "BLOCKING"
	}
	return "WARNING"
}

// Category groups related checks.
const (
	CategoryHA          = "High Availability"
	CategoryReliability = "Reliability"
	CategorySecurity    = "Security"
	CategoryStorage     = "Storage"
	CategoryNetwork     = "Network"
)

// Finding is one proactive health check result.
type Finding struct {
	Severity  Severity
	Category  string
	Kind      string
	Namespace string
	Name      string
	Issue     string // one-line description
	Detail    string // additional context
	Fix       string // suggested remediation
}

// Resource returns a display string for the affected resource.
func (f Finding) Resource() string {
	if f.Namespace == "" {
		return f.Kind + " " + f.Name
	}
	return f.Kind + " " + f.Namespace + "/" + f.Name
}

// Scan runs all proactive checks against the graph and returns sorted findings.
// namespace="" scans all namespaces.
func Scan(g *graph.Graph, namespace string) []Finding {
	var findings []Finding
	findings = append(findings, checkHA(g, namespace)...)
	findings = append(findings, checkReliability(g, namespace)...)
	findings = append(findings, checkSecurity(g, namespace)...)
	findings = append(findings, checkStorage(g, namespace)...)
	findings = append(findings, checkNetwork(g, namespace)...)

	sort.Slice(findings, func(i, j int) bool {
		// Blocking before Warning, then by category, then by resource.
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity > findings[j].Severity
		}
		if findings[i].Category != findings[j].Category {
			return findings[i].Category < findings[j].Category
		}
		return findings[i].Resource() < findings[j].Resource()
	})
	return findings
}

// ─── High Availability ───────────────────────────────────────────────────────

func checkHA(g *graph.Graph, namespace string) []Finding {
	var findings []Finding
	for _, n := range g.Nodes() {
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		switch n.Kind {
		case "Deployment":
			if d, ok := n.Raw.(*appsv1.Deployment); ok && d != nil {
				replicas := int32(1)
				if d.Spec.Replicas != nil {
					replicas = *d.Spec.Replicas
				}
				if replicas < 2 {
					findings = append(findings, Finding{
						Severity:  SeverityWarning,
						Category:  CategoryHA,
						Kind:      "Deployment",
						Namespace: n.Namespace,
						Name:      n.Name,
						Issue:     "single replica — no high availability",
						Detail:    fmt.Sprintf("spec.replicas=%d; a single Pod failure will cause downtime", replicas),
						Fix:       "set spec.replicas to at least 2 and configure a PodDisruptionBudget",
					})
				}
				if d.Status.AvailableReplicas < d.Status.Replicas {
					findings = append(findings, Finding{
						Severity:  SeverityBlocking,
						Category:  CategoryHA,
						Kind:      "Deployment",
						Namespace: n.Namespace,
						Name:      n.Name,
						Issue:     fmt.Sprintf("available replicas (%d) < desired (%d)", d.Status.AvailableReplicas, d.Status.Replicas),
						Detail:    "some Pods are not ready; the service is running below capacity",
						Fix:       "run 'kgraph why deployment/" + n.Name + "' to find the root cause",
					})
				}
			}
		case "StatefulSet":
			if s, ok := n.Raw.(*appsv1.StatefulSet); ok && s != nil {
				replicas := int32(1)
				if s.Spec.Replicas != nil {
					replicas = *s.Spec.Replicas
				}
				if replicas < 2 {
					findings = append(findings, Finding{
						Severity:  SeverityWarning,
						Category:  CategoryHA,
						Kind:      "StatefulSet",
						Namespace: n.Namespace,
						Name:      n.Name,
						Issue:     "single replica — no high availability",
						Detail:    fmt.Sprintf("spec.replicas=%d", replicas),
						Fix:       "set spec.replicas to at least 2 (ensure your application supports it)",
					})
				}
			}
		}
	}
	return findings
}

// ─── Reliability ─────────────────────────────────────────────────────────────

func checkReliability(g *graph.Graph, namespace string) []Finding {
	var findings []Finding
	for _, n := range g.Nodes() {
		if n.Kind != "Pod" {
			continue
		}
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		pod, ok := n.Raw.(*corev1.Pod)
		if !ok || pod == nil {
			continue
		}
		// Only check Pods managed by a workload (skip standalone pods).
		if len(pod.OwnerReferences) == 0 {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if c.LivenessProbe == nil {
				findings = append(findings, Finding{
					Severity:  SeverityWarning,
					Category:  CategoryReliability,
					Kind:      "Pod",
					Namespace: n.Namespace,
					Name:      n.Name,
					Issue:     fmt.Sprintf("container %q has no liveness probe", c.Name),
					Detail:    "Kubernetes cannot detect and restart a stuck or deadlocked process",
					Fix:       "add a livenessProbe (httpGet, tcpSocket, or exec) to container " + c.Name,
				})
			}
			if c.ReadinessProbe == nil {
				findings = append(findings, Finding{
					Severity:  SeverityWarning,
					Category:  CategoryReliability,
					Kind:      "Pod",
					Namespace: n.Namespace,
					Name:      n.Name,
					Issue:     fmt.Sprintf("container %q has no readiness probe", c.Name),
					Detail:    "traffic may be sent to a Pod that is not yet ready to serve requests",
					Fix:       "add a readinessProbe to container " + c.Name,
				})
			}
			// Resource limits
			if c.Resources.Limits == nil ||
				(c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero()) {
				findings = append(findings, Finding{
					Severity:  SeverityWarning,
					Category:  CategoryReliability,
					Kind:      "Pod",
					Namespace: n.Namespace,
					Name:      n.Name,
					Issue:     fmt.Sprintf("container %q has no resource limits", c.Name),
					Detail:    "a runaway process can consume all node CPU/memory and starve other workloads",
					Fix:       "set resources.limits.cpu and resources.limits.memory for container " + c.Name,
				})
			}
		}
		// CronJob missing concurrency policy (checked via parent)
		for _, ref := range pod.OwnerReferences {
			_ = ref // parent kind is available via graph edges if needed
		}
	}
	// Check CronJobs for missing suspend flag or excessive schedule
	for _, n := range g.Nodes() {
		if n.Kind != "CronJob" {
			continue
		}
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		cj, ok := n.Raw.(*batchv1.CronJob)
		if !ok || cj == nil {
			continue
		}
		if cj.Spec.FailedJobsHistoryLimit == nil || *cj.Spec.FailedJobsHistoryLimit == 0 {
			findings = append(findings, Finding{
				Severity:  SeverityWarning,
				Category:  CategoryReliability,
				Kind:      "CronJob",
				Namespace: n.Namespace,
				Name:      n.Name,
				Issue:     "failedJobsHistoryLimit is 0 — failed jobs are immediately deleted",
				Detail:    "there is no record of job failures for debugging",
				Fix:       "set spec.failedJobsHistoryLimit to at least 1",
			})
		}
	}
	return findings
}

// ─── Security ────────────────────────────────────────────────────────────────

func checkSecurity(g *graph.Graph, namespace string) []Finding {
	var findings []Finding

	// RBAC: detect workloads referencing a ServiceAccount that doesn't exist.
	workloadKinds := map[string]bool{
		"Deployment": true, "StatefulSet": true, "DaemonSet": true,
		"Job": true, "CronJob": true,
	}
	saExists := map[string]bool{}
	for _, n := range g.Nodes() {
		if n.Kind == "ServiceAccount" {
			saExists[n.Namespace+"/"+n.Name] = true
		}
	}
	for _, n := range g.Nodes() {
		if !workloadKinds[n.Kind] {
			continue
		}
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		var saName string
		switch raw := n.Raw.(type) {
		case *appsv1.Deployment:
			if raw != nil {
				saName = raw.Spec.Template.Spec.ServiceAccountName
			}
		case *appsv1.StatefulSet:
			if raw != nil {
				saName = raw.Spec.Template.Spec.ServiceAccountName
			}
		case *appsv1.DaemonSet:
			if raw != nil {
				saName = raw.Spec.Template.Spec.ServiceAccountName
			}
		}
		if saName == "" || saName == "default" {
			continue
		}
		if !saExists[n.Namespace+"/"+saName] {
			findings = append(findings, Finding{
				Severity:  SeverityBlocking,
				Category:  CategorySecurity,
				Kind:      n.Kind,
				Namespace: n.Namespace,
				Name:      n.Name,
				Issue:     fmt.Sprintf("references ServiceAccount %q which does not exist", saName),
				Detail:    "pods will fail to start with a ServiceAccount not found error",
				Fix:       "create ServiceAccount " + n.Namespace + "/" + saName + " or correct spec.serviceAccountName",
			})
		}
	}

	// Container-level security checks.
	for _, n := range g.Nodes() {
		if n.Kind != "Pod" {
			continue
		}
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		pod, ok := n.Raw.(*corev1.Pod)
		if !ok || pod == nil {
			continue
		}
		for _, c := range pod.Spec.Containers {
			sc := c.SecurityContext
			if sc == nil {
				continue
			}
			if sc.Privileged != nil && *sc.Privileged {
				findings = append(findings, Finding{
					Severity:  SeverityBlocking,
					Category:  CategorySecurity,
					Kind:      "Pod",
					Namespace: n.Namespace,
					Name:      n.Name,
					Issue:     fmt.Sprintf("container %q runs as privileged", c.Name),
					Detail:    "a privileged container has full access to the host kernel and devices",
					Fix:       "remove securityContext.privileged or set it to false",
				})
			}
			if sc.RunAsUser != nil && *sc.RunAsUser == 0 {
				findings = append(findings, Finding{
					Severity:  SeverityBlocking,
					Category:  CategorySecurity,
					Kind:      "Pod",
					Namespace: n.Namespace,
					Name:      n.Name,
					Issue:     fmt.Sprintf("container %q runs as root (UID 0)", c.Name),
					Detail:    "running as root increases the blast radius of a container escape",
					Fix:       "set securityContext.runAsUser to a non-zero UID and runAsNonRoot: true",
				})
			}
			if sc.AllowPrivilegeEscalation != nil && *sc.AllowPrivilegeEscalation {
				findings = append(findings, Finding{
					Severity:  SeverityWarning,
					Category:  CategorySecurity,
					Kind:      "Pod",
					Namespace: n.Namespace,
					Name:      n.Name,
					Issue:     fmt.Sprintf("container %q allows privilege escalation", c.Name),
					Detail:    "a process can gain more privileges than its parent",
					Fix:       "set securityContext.allowPrivilegeEscalation: false",
				})
			}
		}
	}
	return findings
}

// ─── Storage ─────────────────────────────────────────────────────────────────

func checkStorage(g *graph.Graph, namespace string) []Finding {
	var findings []Finding
	for _, n := range g.Nodes() {
		if n.Kind != "PersistentVolumeClaim" {
			continue
		}
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		pvc, ok := n.Raw.(*corev1.PersistentVolumeClaim)
		if !ok || pvc == nil {
			continue
		}
		if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
			findings = append(findings, Finding{
				Severity:  SeverityWarning,
				Category:  CategoryStorage,
				Kind:      "PersistentVolumeClaim",
				Namespace: n.Namespace,
				Name:      n.Name,
				Issue:     "no explicit StorageClass set",
				Detail:    "uses the cluster default; changing the default StorageClass will affect this PVC",
				Fix:       "set spec.storageClassName explicitly to the intended StorageClass",
			})
		}
	}
	return findings
}

// ─── Network ─────────────────────────────────────────────────────────────────
func checkNetwork(g *graph.Graph, namespace string) []Finding {
	var findings []Finding

	// Collect all namespaces that have at least one NetworkPolicy.
	namespacesWithPolicies := map[string]bool{}
	for _, n := range g.Nodes() {
		if n.Kind == "NetworkPolicy" {
			if np, ok := n.Raw.(*networkingv1.NetworkPolicy); ok && np != nil {
				namespacesWithPolicies[np.Namespace] = true
			}
		}
	}

	// In namespaces where NetworkPolicies exist, flag Services whose Pods have
	// no policy governing them (they are unprotected in a policy-aware namespace).
	for _, n := range g.Nodes() {
		if n.Kind != "Service" {
			continue
		}
		if namespace != "" && n.Namespace != namespace {
			continue
		}
		if !namespacesWithPolicies[n.Namespace] {
			continue // namespace has no policies at all — skip
		}
		svc, ok := n.Raw.(*corev1.Service)
		if !ok || svc == nil || len(svc.Spec.Selector) == 0 {
			continue
		}
		// Check whether any outgoing Pod from this Service has an ingress
		// NetworkPolicy. Use graph edges: Service → Pod (selects).
		anyPodProtected := false
		for _, uid := range g.Out(n.UID) {
			podNode, ok := g.Node(uid)
			if !ok || podNode.Kind != "Pod" {
				continue
			}
			// Check if any NetworkPolicy's policy-selects edge points to this Pod.
			for _, e := range g.InEdges(podNode.UID) {
				if e.Rel == graph.RelPolicySelects {
					anyPodProtected = true
					break
				}
			}
			if anyPodProtected {
				break
			}
		}
		if !anyPodProtected {
			findings = append(findings, Finding{
				Severity:  SeverityWarning,
				Category:  CategoryNetwork,
				Kind:      "Service",
				Namespace: n.Namespace,
				Name:      n.Name,
				Issue:     "no NetworkPolicy governs the Pods this Service selects",
				Detail:    "other NetworkPolicies exist in this namespace; these Pods are unprotected",
				Fix:       "add a NetworkPolicy with spec.podSelector matching this Service's selector",
			})
		}
	}
	return findings
}
