// Package network implements deterministic network reachability analysis over
// Kubernetes NetworkPolicies. It is purely analytical — no cluster mutations,
// no network probes, no AI.
//
// Algorithm summary:
//
//	For a source Pod → destination Pod connection on a given port:
//	  1. Collect policies in the source namespace whose spec.podSelector matches
//	     the source Pod. Among them, identify those that restrict egress.
//	     If none restrict egress → egress is unrestricted (open).
//	  2. Collect policies in the destination namespace whose spec.podSelector
//	     matches the destination Pod. Among them, identify those that restrict
//	     ingress. If none restrict ingress → ingress is unrestricted (open).
//	  3. For restricted egress: the connection is allowed iff at least one
//	     matching policy has an egress rule whose "to" selector matches the
//	     destination Pod and whose port matches.
//	  4. For restricted ingress: the connection is allowed iff at least one
//	     matching policy has an ingress rule whose "from" selector matches the
//	     source Pod and whose port matches.
//	  5. Final verdict: ALLOWED iff both egress and ingress permit.
package network

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/apimachinery/pkg/labels"
)

// Verdict is the reachability decision.
type Verdict int

const (
	Allowed Verdict = iota // connection is permitted
	Blocked                // at least one policy blocks the connection
	Unknown                // an ipBlock rule prevents deterministic evaluation
)

func (v Verdict) String() string {
	switch v {
	case Allowed:
		return "ALLOWED"
	case Blocked:
		return "BLOCKED"
	default:
		return "UNKNOWN"
	}
}

// PolicyEffect records one NetworkPolicy's evaluation for a direction.
type PolicyEffect struct {
	Name      string
	Namespace string
	// Selects is true when this policy's podSelector matched the relevant Pod.
	Selects bool
	// Restricts is true when this policy actually limits traffic in this direction
	// (i.e. the direction is in spec.policyTypes or implied).
	Restricts bool
	// Allows is true when this policy has at least one rule permitting the connection.
	Allows bool
	// MatchedRuleIndex is the index of the first matching rule (-1 = no match).
	MatchedRuleIndex int
	// HasIPBlock indicates one or more rules use ipBlock, which cannot be
	// evaluated without knowing the actual Pod IP addresses.
	HasIPBlock bool
}

// HalfVerdict is the result for one traffic direction (egress or ingress).
type HalfVerdict struct {
	Open    bool           // no policies restrict this direction → open
	Allowed bool           // at least one rule permits the connection
	Effects []PolicyEffect // per-policy evaluations
}

// Result is the full reachability analysis for one src→dst connection.
type Result struct {
	SrcPod corev1.Pod
	DstPod corev1.Pod
	Port   int32           // 0 = any port
	Proto  corev1.Protocol // "" defaults to TCP

	Verdict Verdict
	Egress  HalfVerdict
	Ingress HalfVerdict
	Notes   []string
}

// Check evaluates whether srcPod can reach dstPod on the given port/protocol.
// Pass port=0 to check reachability on any port. policies must include all
// NetworkPolicies in both pods' namespaces; namespaceLabels maps namespace name
// to its labels (required for namespaceSelector rules).
func Check(
	srcPod, dstPod corev1.Pod,
	port int32,
	proto corev1.Protocol,
	policies []networkingv1.NetworkPolicy,
	namespaceLabels map[string]map[string]string,
) Result {
	if proto == "" {
		proto = corev1.ProtocolTCP
	}

	r := Result{
		SrcPod: srcPod,
		DstPod: dstPod,
		Port:   port,
		Proto:  proto,
	}

	r.Egress = evalEgress(srcPod, dstPod, port, proto, policies, namespaceLabels)
	r.Ingress = evalIngress(srcPod, dstPod, port, proto, policies, namespaceLabels)

	switch {
	case !r.Egress.Open && !r.Egress.Allowed:
		r.Verdict = Blocked
	case !r.Ingress.Open && !r.Ingress.Allowed:
		r.Verdict = Blocked
	default:
		// Check for unknown (ipBlock) cases that prevent a definitive answer.
		for _, e := range r.Egress.Effects {
			if e.HasIPBlock && !e.Allows {
				r.Verdict = Unknown
				r.Notes = append(r.Notes, fmt.Sprintf(
					"policy %s/%s uses ipBlock which cannot be evaluated without live Pod IPs",
					e.Namespace, e.Name))
			}
		}
		for _, e := range r.Ingress.Effects {
			if e.HasIPBlock && !e.Allows {
				r.Verdict = Unknown
				r.Notes = append(r.Notes, fmt.Sprintf(
					"policy %s/%s uses ipBlock which cannot be evaluated without live Pod IPs",
					e.Namespace, e.Name))
			}
		}
		if r.Verdict != Unknown {
			r.Verdict = Allowed
		}
	}
	return r
}

// NamespaceLabelsFromList builds the namespaceLabels map from a list of Namespace objects.
func NamespaceLabelsFromList(namespaces []corev1.Namespace) map[string]map[string]string {
	m := make(map[string]map[string]string, len(namespaces))
	for _, ns := range namespaces {
		m[ns.Name] = ns.Labels
	}
	return m
}

// evalEgress evaluates whether egress from srcPod to dstPod is permitted.
func evalEgress(
	srcPod, dstPod corev1.Pod,
	port int32, proto corev1.Protocol,
	policies []networkingv1.NetworkPolicy,
	nsLabels map[string]map[string]string,
) HalfVerdict {
	hv := HalfVerdict{Open: true}

	for _, np := range policies {
		if np.Namespace != srcPod.Namespace {
			continue
		}
		effect := PolicyEffect{
			Name: np.Name, Namespace: np.Namespace,
			MatchedRuleIndex: -1,
		}

		// Does this policy select srcPod?
		sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
		if err != nil {
			continue
		}
		effect.Selects = sel.Matches(labels.Set(srcPod.Labels))
		if !effect.Selects {
			continue
		}

		// Does this policy restrict egress?
		effect.Restricts = restrictsEgress(np)
		if !effect.Restricts {
			continue
		}
		hv.Open = false // at least one policy restricts egress from srcPod

		// Check each egress rule.
		for idx, rule := range np.Spec.Egress {
			matched, hasIPBlock := egressRuleMatches(rule, dstPod, port, proto, np.Namespace, nsLabels)
			if hasIPBlock {
				effect.HasIPBlock = true
			}
			if matched {
				effect.Allows = true
				effect.MatchedRuleIndex = idx
				break
			}
		}
		hv.Effects = append(hv.Effects, effect)
	}

	if !hv.Open {
		for _, e := range hv.Effects {
			if e.Allows {
				hv.Allowed = true
				break
			}
		}
	}
	return hv
}

// evalIngress evaluates whether ingress to dstPod from srcPod is permitted.
func evalIngress(
	srcPod, dstPod corev1.Pod,
	port int32, proto corev1.Protocol,
	policies []networkingv1.NetworkPolicy,
	nsLabels map[string]map[string]string,
) HalfVerdict {
	hv := HalfVerdict{Open: true}

	for _, np := range policies {
		if np.Namespace != dstPod.Namespace {
			continue
		}
		effect := PolicyEffect{
			Name: np.Name, Namespace: np.Namespace,
			MatchedRuleIndex: -1,
		}

		sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
		if err != nil {
			continue
		}
		effect.Selects = sel.Matches(labels.Set(dstPod.Labels))
		if !effect.Selects {
			continue
		}

		effect.Restricts = restrictsIngress(np)
		if !effect.Restricts {
			continue
		}
		hv.Open = false

		for idx, rule := range np.Spec.Ingress {
			matched, hasIPBlock := ingressRuleMatches(rule, srcPod, port, proto, np.Namespace, nsLabels)
			if hasIPBlock {
				effect.HasIPBlock = true
			}
			if matched {
				effect.Allows = true
				effect.MatchedRuleIndex = idx
				break
			}
		}
		hv.Effects = append(hv.Effects, effect)
	}

	if !hv.Open {
		for _, e := range hv.Effects {
			if e.Allows {
				hv.Allowed = true
				break
			}
		}
	}
	return hv
}

// restrictsEgress returns true when the policy actively restricts egress.
// Per the Kubernetes spec: a policy restricts egress if "Egress" appears in
// spec.policyTypes, or if spec.policyTypes is empty and spec.egress is present.
func restrictsEgress(np networkingv1.NetworkPolicy) bool {
	for _, pt := range np.Spec.PolicyTypes {
		if pt == networkingv1.PolicyTypeEgress {
			return true
		}
	}
	// If policyTypes is empty, egress is only restricted when spec.egress is set.
	return len(np.Spec.PolicyTypes) == 0 && len(np.Spec.Egress) > 0
}

// restrictsIngress returns true when the policy actively restricts ingress.
// Per the spec: ingress is always implied when policyTypes is empty.
func restrictsIngress(np networkingv1.NetworkPolicy) bool {
	if len(np.Spec.PolicyTypes) == 0 {
		return true // ingress is always implied
	}
	for _, pt := range np.Spec.PolicyTypes {
		if pt == networkingv1.PolicyTypeIngress {
			return true
		}
	}
	return false
}

// egressRuleMatches returns (matched, hasIPBlock) for one egress rule against
// the destination Pod on the given port.
func egressRuleMatches(
	rule networkingv1.NetworkPolicyEgressRule,
	dstPod corev1.Pod,
	port int32, proto corev1.Protocol,
	policyNs string,
	nsLabels map[string]map[string]string,
) (bool, bool) {
	// Empty rule.ports → all ports; empty rule.to → all destinations.
	portOK := len(rule.Ports) == 0 || anyPortMatches(rule.Ports, port, proto, dstPod)
	if !portOK {
		return false, false
	}
	if len(rule.To) == 0 {
		return true, false
	}
	for _, peer := range rule.To {
		matched, hasIPBlock := peerMatchesPod(peer, dstPod, policyNs, nsLabels)
		if hasIPBlock {
			return false, true
		}
		if matched {
			return true, false
		}
	}
	return false, false
}

// ingressRuleMatches returns (matched, hasIPBlock) for one ingress rule against
// the source Pod on the given port.
func ingressRuleMatches(
	rule networkingv1.NetworkPolicyIngressRule,
	srcPod corev1.Pod,
	port int32, proto corev1.Protocol,
	policyNs string,
	nsLabels map[string]map[string]string,
) (bool, bool) {
	portOK := len(rule.Ports) == 0 || anyPortMatches(rule.Ports, port, proto, srcPod)
	if !portOK {
		return false, false
	}
	if len(rule.From) == 0 {
		return true, false
	}
	for _, peer := range rule.From {
		matched, hasIPBlock := peerMatchesPod(peer, srcPod, policyNs, nsLabels)
		if hasIPBlock {
			return false, true
		}
		if matched {
			return true, false
		}
	}
	return false, false
}

// peerMatchesPod checks whether a NetworkPolicyPeer matches a given Pod.
// Returns (matched, hasIPBlock). An ipBlock peer cannot be resolved
// deterministically without knowing the Pod's live IP address.
func peerMatchesPod(
	peer networkingv1.NetworkPolicyPeer,
	pod corev1.Pod,
	policyNs string,
	nsLabels map[string]map[string]string,
) (bool, bool) {
	if peer.IPBlock != nil {
		return false, true
	}

	// Namespace check: if no namespaceSelector, the pod must be in the same
	// namespace as the policy.
	var nsMatch bool
	if peer.NamespaceSelector == nil {
		nsMatch = pod.Namespace == policyNs
	} else {
		nsSel, err := metav1.LabelSelectorAsSelector(peer.NamespaceSelector)
		if err != nil {
			return false, false
		}
		podNsLabels := nsLabels[pod.Namespace]
		nsMatch = nsSel.Matches(labels.Set(podNsLabels))
	}
	if !nsMatch {
		return false, false
	}

	// Pod label check.
	if peer.PodSelector == nil {
		return true, false
	}
	podSel, err := metav1.LabelSelectorAsSelector(peer.PodSelector)
	if err != nil {
		return false, false
	}
	return podSel.Matches(labels.Set(pod.Labels)), false
}

// anyPortMatches returns true if any NetworkPolicyPort in the list matches
// the given port and protocol. Named ports are resolved against the Pod spec.
func anyPortMatches(
	ports []networkingv1.NetworkPolicyPort,
	port int32, proto corev1.Protocol,
	pod corev1.Pod,
) bool {
	for _, np := range ports {
		if np.Protocol != nil && *np.Protocol != proto {
			continue
		}
		if np.Port == nil {
			return true // matches all ports on this protocol
		}
		switch np.Port.Type {
		case intstr.Int:
			if int32(np.Port.IntValue()) == port {
				return true
			}
		case intstr.String:
			// Named port: resolve from Pod's container ports.
			portName := np.Port.String()
			for _, c := range pod.Spec.Containers {
				for _, cp := range c.Ports {
					if cp.Name == portName && cp.ContainerPort == port {
						return true
					}
				}
			}
		}
	}
	return false
}

// Suggestion generates a human-readable suggestion for how to fix a blocked connection.
func Suggestion(r Result) string {
	var parts []string
	portDesc := "any port"
	if r.Port > 0 {
		portDesc = fmt.Sprintf("port %d/%s", r.Port, r.Proto)
	}

	if !r.Egress.Open && !r.Egress.Allowed {
		for _, e := range r.Egress.Effects {
			if e.Restricts && !e.Allows {
				parts = append(parts, fmt.Sprintf(
					"add an egress rule to NetworkPolicy %q in namespace %q allowing traffic to %s/%s on %s",
					e.Name, e.Namespace, r.DstPod.Namespace, r.DstPod.Name, portDesc))
				break
			}
		}
	}
	if !r.Ingress.Open && !r.Ingress.Allowed {
		for _, e := range r.Ingress.Effects {
			if e.Restricts && !e.Allows {
				parts = append(parts, fmt.Sprintf(
					"add an ingress rule to NetworkPolicy %q in namespace %q allowing traffic from %s/%s on %s",
					e.Name, e.Namespace, r.SrcPod.Namespace, r.SrcPod.Name, portDesc))
				break
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

// FormatResult renders a Result as a multi-line terminal string. Color codes
// are handled by the caller (lipgloss).
func FormatResult(r Result) string {
	var b strings.Builder

	portDesc := "any port"
	if r.Port > 0 {
		portDesc = fmt.Sprintf("port %d/%s", r.Port, r.Proto)
	}
	fmt.Fprintf(&b, "  Source:       Pod %s/%s\n", r.SrcPod.Namespace, r.SrcPod.Name)
	fmt.Fprintf(&b, "  Destination:  Pod %s/%s   %s\n\n", r.DstPod.Namespace, r.DstPod.Name, portDesc)

	writeHalf := func(label string, hv HalfVerdict, checkPod corev1.Pod) {
		if hv.Open {
			fmt.Fprintf(&b, "  %s:  no NetworkPolicies restrict this direction → unrestricted\n", label)
			return
		}
		fmt.Fprintf(&b, "  %s:\n", label)
		for _, e := range hv.Effects {
			allowed := "✖ no rule permits this connection"
			if e.Allows {
				allowed = fmt.Sprintf("✓ rule %d permits", e.MatchedRuleIndex)
			} else if e.HasIPBlock {
				allowed = "? ipBlock rule — cannot evaluate without live Pod IP"
			}
			fmt.Fprintf(&b, "    NetworkPolicy %s/%s  [selects %s/%s]\n",
				e.Namespace, e.Name, checkPod.Namespace, checkPod.Name)
			fmt.Fprintf(&b, "      %s\n", allowed)
		}
	}

	writeHalf("Egress (from src)", r.Egress, r.SrcPod)
	writeHalf("Ingress (to dst)", r.Ingress, r.DstPod)

	b.WriteString("\n")
	return b.String()
}

// SortPolicies returns a deterministically sorted copy for stable output.
func SortPolicies(policies []networkingv1.NetworkPolicy) []networkingv1.NetworkPolicy {
	out := make([]networkingv1.NetworkPolicy, len(policies))
	copy(out, policies)
	sort.Slice(out, func(i, j int) bool {
		ki := out[i].Namespace + "/" + out[i].Name
		kj := out[j].Namespace + "/" + out[j].Name
		return ki < kj
	})
	return out
}
