package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/network"
)

var (
	canReachPort      int32
	canReachProtocol  string
	canReachNamespace string
)

var canReachCmd = &cobra.Command{
	Use:   "can-reach <pod/name> <pod/name|svc/name>",
	Short: "Trace whether a Pod can reach a destination given NetworkPolicies",
	Long: `Evaluates Kubernetes NetworkPolicies to determine whether a source Pod is
allowed to reach a destination Pod or Service without any network probes.

Analysis covers:
  • Egress policies on the source Pod
  • Ingress policies on destination Pod(s)
  • Same-namespace podSelector rules
  • Cross-namespace namespaceSelector rules
  • Port and protocol restrictions

ipBlock rules (CIDR ranges) are flagged as UNKNOWN because they require live
Pod IP addresses which are not available from the collected graph.

Examples:
  kgraph can-reach pod/frontend pod/payments -n default
  kgraph can-reach pod/web svc/api --port 8080 --protocol TCP`,
	Args: cobra.ExactArgs(2),
	RunE: runCanReach,
}

func runCanReach(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	proto := corev1.Protocol(strings.ToUpper(canReachProtocol))

	ctxName := kubeContext
	if ctxName == "" {
		cfg, err := loadKubeconfig()
		if err != nil {
			return fmt.Errorf("load kubeconfig: %w", err)
		}
		ctxName = cfg.CurrentContext
	}
	c, err := collector.New(kubeconfig, ctxName)
	if err != nil {
		return err
	}
	res, err := c.Collect(ctx)
	if err != nil {
		return err
	}
	g := graph.Build(res)

	ns := canReachNamespace
	if ns == "" {
		ns = "default"
	}

	// Resolve source — must be a Pod.
	srcPod, err := resolvePod(g, res, args[0], ns)
	if err != nil {
		return fmt.Errorf("source: %w", err)
	}

	// Resolve destination — Pod or Service (→ endpoint pods).
	dstPods, dstLabel, err := resolveDest(g, res, args[1], ns)
	if err != nil {
		return fmt.Errorf("destination: %w", err)
	}

	// Build namespace label map for cross-namespace rules.
	nsLabels := network.NamespaceLabelsFromList(res.Namespaces)
	policies := network.SortPolicies(res.NetworkPolicies)

	// Style helpers.
	title := lipgloss.NewStyle().Bold(true)
	allowed := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	blocked := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	unknown := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	section := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	faint := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	portDesc := "any port"
	if canReachPort > 0 {
		portDesc = fmt.Sprintf("port %d/%s", canReachPort, proto)
	}

	fmt.Fprintf(os.Stdout, "\n%s\n", title.Render(fmt.Sprintf(
		"Reachability check  (%s)  context: %s", portDesc, ctxName)))
	fmt.Fprintf(os.Stdout, "  Source:       Pod %s/%s\n", srcPod.Namespace, srcPod.Name)
	fmt.Fprintf(os.Stdout, "  Destination:  %s\n\n", dstLabel)

	// Evaluate reachability for each destination Pod.
	anyBlocked, anyAllowed := false, false
	separator := strings.Repeat("─", 64)

	for i, dst := range dstPods {
		if len(dstPods) > 1 {
			fmt.Fprintf(os.Stdout, "  %s\n", faint.Render(fmt.Sprintf(
				"%s  [%d/%d] %s/%s", separator, i+1, len(dstPods), dst.Namespace, dst.Name)))
		}
		r := network.Check(*srcPod, dst, canReachPort, proto, policies, nsLabels)

		verdictStr := allowed.Render("✓ ALLOWED")
		switch r.Verdict {
		case network.Blocked:
			verdictStr = blocked.Render("✖ BLOCKED")
			anyBlocked = true
		case network.Unknown:
			verdictStr = unknown.Render("? UNKNOWN (ipBlock)")
		default:
			anyAllowed = true
		}

		fmt.Fprintf(os.Stdout, "  Verdict:  %s\n\n", verdictStr)

		printHalf := func(label string, hv network.HalfVerdict, pod corev1.Pod) {
			if hv.Open {
				fmt.Fprintf(os.Stdout, "  %s\n    %s\n",
					section.Render(label),
					faint.Render("no NetworkPolicies restrict this direction → unrestricted"))
				return
			}
			fmt.Fprintf(os.Stdout, "  %s\n", section.Render(label))
			for _, e := range hv.Effects {
				fmt.Fprintf(os.Stdout, "    NetworkPolicy %s\n",
					title.Render(e.Namespace+"/"+e.Name))
				if e.HasIPBlock {
					fmt.Fprintf(os.Stdout, "      %s\n",
						warn.Render("? ipBlock rule — cannot evaluate without live Pod IP"))
				} else if e.Allows {
					fmt.Fprintf(os.Stdout, "      %s\n",
						allowed.Render(fmt.Sprintf("✓ rule %d permits this connection", e.MatchedRuleIndex)))
				} else {
					fmt.Fprintf(os.Stdout, "      %s\n",
						blocked.Render("✖ no rule permits this connection"))
				}
			}
		}

		printHalf(fmt.Sprintf("Egress (from %s/%s)", srcPod.Namespace, srcPod.Name), r.Egress, *srcPod)
		printHalf(fmt.Sprintf("Ingress (to %s/%s)", dst.Namespace, dst.Name), r.Ingress, dst)

		if sug := network.Suggestion(r); sug != "" {
			fmt.Fprintf(os.Stdout, "\n  %s\n", section.Render("Suggestion"))
			for _, line := range strings.Split(sug, "\n") {
				fmt.Fprintf(os.Stdout, "    %s\n", line)
			}
		}
		if r.Verdict != network.Allowed {
			for _, note := range r.Notes {
				fmt.Fprintf(os.Stdout, "  %s\n", faint.Render("note: "+note))
			}
		}
		fmt.Fprintln(os.Stdout)
	}

	if len(dstPods) > 1 {
		fmt.Fprintf(os.Stdout, "%s\n", faint.Render(separator))
		switch {
		case anyBlocked && anyAllowed:
			fmt.Fprintf(os.Stdout, "  Overall: %s  (some endpoint Pods are reachable)\n",
				unknown.Render("PARTIAL"))
		case anyBlocked:
			fmt.Fprintf(os.Stdout, "  Overall: %s  (no endpoint Pods reachable)\n",
				blocked.Render("✖ BLOCKED"))
		default:
			fmt.Fprintf(os.Stdout, "  Overall: %s  (%d endpoint Pods reachable)\n",
				allowed.Render("✓ ALLOWED"), len(dstPods))
		}
		fmt.Fprintln(os.Stdout)
	}

	if len(policies) == 0 {
		fmt.Fprintf(os.Stdout, "  %s\n",
			faint.Render("ℹ  No NetworkPolicies found in this cluster — all traffic is unrestricted."))
	}

	if anyBlocked {
		os.Exit(1)
	}
	return nil
}

// resolvePod finds a Pod by "pod/name" or a prefix/substring in the given namespace.
// When multiple pods match the same prefix (e.g. replica pods from one Deployment),
// the first one (alphabetically by name) is chosen and the choice is printed so the
// user knows which instance was used. Pass the full pod name to be explicit.
func resolvePod(g *graph.Graph, res graph.Resources, ref, ns string) (*corev1.Pod, error) {
	kind, name, err := splitRef(ref)
	if err != nil {
		return nil, err
	}
	if kind != "" && kind != "pod" {
		return nil, fmt.Errorf("%q is not a Pod reference (use pod/<name>)", ref)
	}
	// Exact match first.
	for i := range res.Pods {
		p := &res.Pods[i]
		if p.Name == name && (p.Namespace == ns || ns == "") {
			return p, nil
		}
	}
	// Prefix/substring match for convenience (e.g. "frontend" → "frontend-abc-xyz").
	var matches []*corev1.Pod
	for i := range res.Pods {
		p := &res.Pods[i]
		if strings.Contains(p.Name, name) && (p.Namespace == ns || ns == "") {
			matches = append(matches, p)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("Pod %q not found in namespace %q", name, ns)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	// Multiple matches: sort by name for determinism, pick the first.
	// For reachability purposes, all pods from the same workload share the
	// same labels, so the result is identical for any replica.
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	chosen := matches[0]
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = m.Name
	}
	fmt.Fprintf(os.Stderr, "note: %q matches %d pods (%s)\n      using %s (reachability is identical for all replicas)\n\n",
		name, len(matches), strings.Join(names, ", "), chosen.Name)
	return chosen, nil
}

// resolveDest resolves a "pod/name" or "svc/name" destination to a list of
// Pods and a display label.
func resolveDest(g *graph.Graph, res graph.Resources, ref, ns string) ([]corev1.Pod, string, error) {
	kind, name, err := splitRef(ref)
	if err != nil {
		return nil, "", err
	}
	switch kind {
	case "", "pod":
		pod, err := resolvePod(g, res, ref, ns)
		if err != nil {
			return nil, "", err
		}
		return []corev1.Pod{*pod}, fmt.Sprintf("Pod %s/%s", pod.Namespace, pod.Name), nil

	case "svc", "service":
		// Find the Service node in the graph.
		var svc *corev1.Service
		for i := range res.Services {
			s := &res.Services[i]
			if s.Name == name && (s.Namespace == ns || ns == "") {
				svc = s
				break
			}
		}
		if svc == nil {
			return nil, "", fmt.Errorf("Service %q not found in namespace %q", name, ns)
		}
		// Find Pods selected by this Service via graph edges.
		svcNode, found := findNode(g, "Service", svc.Namespace, svc.Name)
		if !found {
			return nil, "", fmt.Errorf("Service %s/%s not in graph", svc.Namespace, svc.Name)
		}
		var pods []corev1.Pod
		for _, uid := range g.Out(svcNode.UID) {
			n, ok := g.Node(uid)
			if !ok || n.Kind != "Pod" {
				continue
			}
			if pod, ok := n.Raw.(*corev1.Pod); ok && pod != nil {
				pods = append(pods, *pod)
			}
		}
		if len(pods) == 0 {
			return nil, "", fmt.Errorf("Service %s/%s selects no Pods — check its selector", svc.Namespace, svc.Name)
		}
		label := fmt.Sprintf("Service %s/%s → %d endpoint Pod(s)", svc.Namespace, svc.Name, len(pods))
		return pods, label, nil

	default:
		return nil, "", fmt.Errorf("unsupported destination kind %q (use pod/ or svc/)", kind)
	}
}

func splitRef(ref string) (kind, name string, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", fmt.Errorf("empty reference")
	}
	if i := strings.Index(ref, "/"); i >= 0 {
		return strings.ToLower(ref[:i]), ref[i+1:], nil
	}
	return "", ref, nil
}

func findNode(g *graph.Graph, kind, namespace, name string) (graph.Node, bool) {
	for _, n := range g.Nodes() {
		if n.Kind == kind && n.Namespace == namespace && n.Name == name {
			return n, true
		}
	}
	return graph.Node{}, false
}

func init() {
	canReachCmd.Flags().Int32VarP(&canReachPort, "port", "p", 0,
		"destination port to check (0 = any port)")
	canReachCmd.Flags().StringVar(&canReachProtocol, "protocol", "TCP",
		"protocol to check: TCP, UDP, SCTP")
	canReachCmd.Flags().StringVarP(&canReachNamespace, "namespace", "n", "",
		"namespace for resolving resource names (default: \"default\")")
	rootCmd.AddCommand(canReachCmd)
}
