package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/graph"
)

var (
	logsNamespace string
	logsTail      int64
	logsContainer string
	logsLevel     string // filter: error/warn/info/debug/"" = all
	logsFollow    bool
)

var logsCmd = &cobra.Command{
	Use:   "logs <kind>/<name>",
	Short: "Stream and parse logs with automatic level detection and color coding",
	Long: `Fetches logs from all pods belonging to a resource, parses each line for
log level (ERROR / WARN / INFO / DEBUG / TRACE), and renders them with color
coding. Supports structured JSON logs, logrus/zap text format, and plain text.

Filter by level with --level. Works for any resource that owns pods:
  Deployment, StatefulSet, DaemonSet, Job — shows logs for all pods.
  Pod — shows logs for that pod directly.

Examples:
  kgraph logs deployment/payments -n default
  kgraph logs pod/payments-abc-xyz --level error
  kgraph logs deployment/api --tail 500 --container api`,
	Args: cobra.ExactArgs(1),
	RunE: runLogs,
}

var (
	levelStyles = map[string]lipgloss.Style{
		"FATAL":   lipgloss.NewStyle().Foreground(lipgloss.Color("197")).Bold(true),
		"ERROR":   lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
		"WARN":    lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
		"INFO":    lipgloss.NewStyle().Foreground(lipgloss.Color("75")),
		"DEBUG":   lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		"TRACE":   lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
		"UNKNOWN": lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	}
	levelBadges = map[string]string{
		"FATAL":   "FAT",
		"ERROR":   "ERR",
		"WARN":    "WRN",
		"INFO":    "INF",
		"DEBUG":   "DBG",
		"TRACE":   "TRC",
		"UNKNOWN": "---",
	}
)

func runLogs(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	ctxName, res, err := buildResources(ctx)
	if err != nil {
		return err
	}

	g := graph.Build(res)
	ref, refErr := parseRef(args[0])
	if refErr != nil {
		return refErr
	}
	node, findErr := resolveNode(g, ref)
	if findErr != nil {
		return findErr
	}

	restConfig, err := collector.RESTConfig(kubeconfig, ctxName)
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	// Collect pods.
	type podTarget struct{ ns, name string }
	var pods []podTarget
	if node.Kind == "Pod" {
		pods = append(pods, podTarget{node.Namespace, node.Name})
	} else {
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
			if !ok {
				continue
			}
			if n.Kind == "Pod" {
				pods = append(pods, podTarget{n.Namespace, n.Name})
				continue
			}
			queue = append(queue, g.Out(uid)...)
		}
	}
	if len(pods) == 0 {
		fmt.Fprintf(os.Stdout, "no pods found for %s\n", args[0])
		return nil
	}

	filterLevel := strings.ToUpper(strings.TrimSpace(logsLevel))
	title := lipgloss.NewStyle().Bold(true)
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	for _, pod := range pods {
		// Determine containers.
		var containers []string
		if logsContainer != "" {
			containers = []string{logsContainer}
		} else {
			podObj, _ := client.CoreV1().Pods(pod.ns).Get(ctx, pod.name, metav1.GetOptions{})
			if podObj != nil {
				for _, c := range podObj.Spec.Containers {
					containers = append(containers, c.Name)
				}
			}
			if len(containers) == 0 {
				containers = []string{""}
			}
		}

		for _, cname := range containers {
			header := pod.ns + "/" + pod.name
			if cname != "" {
				header += " [" + cname + "]"
			}
			fmt.Fprintf(os.Stdout, "\n%s\n%s\n",
				title.Render("── "+header+" ──"),
				sep.Render(strings.Repeat("─", 60)))

			opts := &corev1.PodLogOptions{
				TailLines: &logsTail,
				Follow:    logsFollow,
			}
			if cname != "" {
				opts.Container = cname
			}
			fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			stream, streamErr := client.CoreV1().Pods(pod.ns).GetLogs(pod.name, opts).Stream(fetchCtx)
			if streamErr != nil {
				cancel()
				fmt.Fprintf(os.Stdout, "%s\n", dim.Render("  error: "+streamErr.Error()))
				continue
			}

			scanner := bufio.NewScanner(io.LimitReader(stream, 4*1024*1024))
			for scanner.Scan() {
				raw := scanner.Text()
				level := detectLogLevel(raw)
				if filterLevel != "" && filterLevel != level {
					continue
				}
				st, ok := levelStyles[level]
				if !ok {
					st = levelStyles["UNKNOWN"]
				}
				badge := levelBadges[level]
				if badge == "" {
					badge = "---"
				}
				fmt.Fprintf(os.Stdout, "%s %s\n", st.Render(badge), raw)
			}
			stream.Close()
			cancel()
		}
	}
	return nil
}

// detectLogLevel parses one log line and returns the level string.
func detectLogLevel(line string) string {
	// kubectl prefix: E/W/I/D/F + 4-digit date
	if len(line) > 5 {
		switch line[0] {
		case 'E', 'F':
			if line[1] >= '0' && line[1] <= '9' {
				return "ERROR"
			}
		case 'W':
			if line[1] >= '0' && line[1] <= '9' {
				return "WARN"
			}
		case 'I':
			if line[1] >= '0' && line[1] <= '9' {
				return "INFO"
			}
		case 'D':
			if line[1] >= '0' && line[1] <= '9' {
				return "DEBUG"
			}
		}
	}
	// JSON structured
	if strings.HasPrefix(strings.TrimSpace(line), "{") {
		var obj map[string]interface{}
		if json.Unmarshal([]byte(line), &obj) == nil {
			for _, k := range []string{"level", "Level", "severity", "lvl"} {
				if v, ok := obj[k].(string); ok {
					return normLevel(v)
				}
			}
		}
	}
	u := strings.ToUpper(line)
	for _, pair := range [][2]string{
		{"FATAL", "FATAL"}, {"CRIT", "FATAL"},
		{"ERROR", "ERROR"}, {" ERR ", "ERROR"}, {"\"ERR\"", "ERROR"},
		{"WARN", "WARN"},
		{"INFO", "INFO"},
		{"DEBUG", "DEBUG"},
		{"TRACE", "TRACE"},
	} {
		if strings.Contains(u, pair[0]) {
			return pair[1]
		}
	}
	return "UNKNOWN"
}

func normLevel(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "FATAL", "CRIT", "CRITICAL":
		return "FATAL"
	case "ERROR", "ERR":
		return "ERROR"
	case "WARN", "WARNING":
		return "WARN"
	case "INFO", "INFORMATION":
		return "INFO"
	case "DEBUG":
		return "DEBUG"
	case "TRACE":
		return "TRACE"
	}
	return "UNKNOWN"
}

func init() {
	logsCmd.Flags().StringVarP(&logsNamespace, "namespace", "n", "", "namespace (default: from context)")
	logsCmd.Flags().Int64Var(&logsTail, "tail", 200, "last N lines per container")
	logsCmd.Flags().StringVar(&logsContainer, "container", "", "specific container (default: all)")
	logsCmd.Flags().StringVar(&logsLevel, "level", "", "filter: error/warn/info/debug (default: all)")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "stream live logs")
	rootCmd.AddCommand(logsCmd)
}
