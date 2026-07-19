package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/health"
	"github.com/anishetty/kgraph/internal/renderer"
)

var networkNamespace string

var networkCmd = &cobra.Command{
	Use:   "network <ingress/name|svc/name>",
	Short: "Trace the full traffic path from Ingress through Service to Pods",
	Long: `Walks the graph from an Ingress or Service to its backing Pods and shows
the health status at every hop. Reveals broken backends, misconfigured selectors,
and Pods that are not serving traffic.

Examples:
  kgraph network ingress/main -n default
  kgraph network svc/api -n production`,
	Args: cobra.ExactArgs(1),
	RunE: runNetwork,
}

func runNetwork(cmd *cobra.Command, args []string) error {
	g, err := buildGraph(cmd.Context())
	if err != nil {
		return err
	}
	ref, err := parseRef(args[0])
	if err != nil {
		return err
	}
	node, err := resolveNode(g, ref)
	if err != nil {
		return err
	}

	// Styles.
	titleSt := lipgloss.NewStyle().Bold(true)
	bad := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	good := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	key := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	faint := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	color := colorEnabled(cmd.OutOrStdout())
	out := os.Stdout

	switch node.Kind {
	case "Ingress":
		ing, ok := node.Raw.(*networkingv1.Ingress)
		if !ok || ing == nil {
			return fmt.Errorf("could not read Ingress raw object")
		}
		fmt.Fprintf(out, "%s %s\n\n", renderer.Icon("Ingress"), titleSt.Render(node.Namespace+"/"+node.Name))

		// Default backend.
		if db := ing.Spec.DefaultBackend; db != nil && db.Service != nil {
			svcName := db.Service.Name
			fmt.Fprintf(out, "  %s\n", key.Render("default backend → "+svcName))
			printServiceHop(out, g, node.Namespace, svcName, "    ", bad, good, warn, faint, color)
		}

		// Rules.
		for _, rule := range ing.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			fmt.Fprintf(out, "  %s\n", key.Render("host: "+host))
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				svcName := ""
				if path.Backend.Service != nil {
					svcName = path.Backend.Service.Name
				}
				pathStr := path.Path
				if pathStr == "" {
					pathStr = "/"
				}
				fmt.Fprintf(out, "    %s %s\n", faint.Render(pathStr), key.Render("→ "+svcName))
				if svcName != "" {
					printServiceHop(out, g, node.Namespace, svcName, "      ", bad, good, warn, faint, color)
				}
			}
		}

	case "Service":
		fmt.Fprintf(out, "%s %s\n\n", renderer.Icon("Service"), titleSt.Render(node.Namespace+"/"+node.Name))
		printServiceHop(out, g, node.Namespace, node.Name, "  ", bad, good, warn, faint, color)

	default:
		return fmt.Errorf("network only supports ingress/<name> and svc/<name>, got %s/%s", strings.ToLower(node.Kind), node.Name)
	}

	return nil
}

// printServiceHop renders one Service and its backing Pods with health indicators.
func printServiceHop(
	out *os.File,
	g *graph.Graph,
	namespace, svcName, indent string,
	bad, good, warn, faint lipgloss.Style,
	color bool,
) {
	svcNode := findServiceNode(g, namespace, svcName)
	if svcNode == nil {
		fmt.Fprintf(out, "%s%s Service %q not found in graph\n",
			indent, bad.Render("✖"), svcName)
		return
	}

	var svc *corev1.Service
	if s, ok := svcNode.Raw.(*corev1.Service); ok {
		svc = s
	}

	selStr := ""
	if svc != nil && len(svc.Spec.Selector) > 0 {
		parts := make([]string, 0, len(svc.Spec.Selector))
		for k, v := range svc.Spec.Selector {
			parts = append(parts, k+"="+v)
		}
		selStr = " selector: " + strings.Join(parts, ", ")
	}

	// Count pods by health.
	var pods []graph.Node
	for _, uid := range g.Out(svcNode.UID) {
		n, ok := g.Node(uid)
		if ok && n.Kind == "Pod" {
			pods = append(pods, n)
		}
	}

	var ready, notReady int
	for _, p := range pods {
		if ok, _ := health.Pod(p); ok {
			ready++
		} else {
			notReady++
		}
	}

	svcStatus := good.Render(fmt.Sprintf("● %d ready", ready))
	if notReady > 0 {
		svcStatus = warn.Render(fmt.Sprintf("⚠ %d ready / %d not ready", ready, notReady))
	}
	if len(pods) == 0 {
		svcStatus = bad.Render("✖ no matching Pods")
	}

	fmt.Fprintf(out, "%s%s %s  %s%s\n",
		indent,
		renderer.Icon("Service"),
		faint.Render(svcName),
		svcStatus,
		faint.Render(selStr))

	for _, p := range pods {
		podHealth, reason := health.Pod(p)
		podStatus := good.Render("● Ready")
		if !podHealth {
			podStatus = bad.Render("✖ " + reason)
		}
		kindColor := renderer.Icon("Pod")
		_ = color
		fmt.Fprintf(out, "%s  %s %s  %s\n",
			indent, kindColor, faint.Render(p.Namespace+"/"+p.Name), podStatus)
	}
}

func findServiceNode(g *graph.Graph, namespace, name string) *graph.Node {
	for _, n := range g.Nodes() {
		if n.Kind == "Service" && n.Namespace == namespace && n.Name == name {
			n := n
			return &n
		}
	}
	return nil
}

func init() {
	networkCmd.Flags().StringVarP(&networkNamespace, "namespace", "n", "",
		"namespace (default: resolved from kubeconfig context)")
	rootCmd.AddCommand(networkCmd)
}
