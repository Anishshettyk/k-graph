package api

// log_yaml.go — /api/yaml and /api/logs handlers.
// YAML: marshal raw node object → cleaned YAML string.
// Logs: fetch pod logs, parse each line for level, return structured entries.

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/graph"
)

// ─── /api/yaml ───────────────────────────────────────────────────────────────

type YAMLResponse struct {
	Context  string `json:"context"`
	Resource string `json:"resource"`
	YAML     string `json:"yaml"`
}

func (h *Handler) handleYAML(w http.ResponseWriter, r *http.Request) {
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

	jsonBytes, err := json.Marshal(node.Raw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "marshal: "+err.Error())
		return
	}
	var obj map[string]interface{}
	if e := json.Unmarshal(jsonBytes, &obj); e == nil {
		if meta, ok := obj["metadata"].(map[string]interface{}); ok {
			delete(meta, "managedFields")
			delete(meta, "resourceVersion")
		}
		jsonBytes, _ = json.Marshal(obj)
	}
	yamlBytes, err := sigsyaml.JSONToYAML(jsonBytes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "yaml: "+err.Error())
		return
	}
	writeJSON(w, YAMLResponse{Context: ctxName, Resource: resource, YAML: string(yamlBytes)})
}

// ─── /api/logs ───────────────────────────────────────────────────────────────

// LogLevel constants (client-visible strings).
const (
	LevelFatal   = "FATAL"
	LevelError   = "ERROR"
	LevelWarn    = "WARN"
	LevelInfo    = "INFO"
	LevelDebug   = "DEBUG"
	LevelTrace   = "TRACE"
	LevelUnknown = "UNKNOWN"
)

// LogEntry is one parsed log line.
type LogEntry struct {
	LineNum   int    `json:"lineNum"`
	Raw       string `json:"raw"`
	Level     string `json:"level"`
	Timestamp string `json:"timestamp,omitempty"`
	Message   string `json:"message,omitempty"`
}

// PodLogResult holds parsed log entries for one (pod, container) pair.
type PodLogResult struct {
	Pod       string     `json:"pod"`
	Container string     `json:"container"`
	Entries   []LogEntry `json:"entries"`
	Error     string     `json:"error,omitempty"`
}

type LogsResponse struct {
	Context string         `json:"context"`
	Pods    []PodLogResult `json:"pods"`
}

var (
	// Ordered from most to least severe; first match wins.
	levelREs = []struct {
		level string
		re    *regexp.Regexp
	}{
		{LevelFatal, regexp.MustCompile(`(?i)(^|[^a-z])(FATAL|CRIT(?:ICAL)?)(:|[^a-z]|$)`)},
		{LevelError, regexp.MustCompile(`(?i)(^|[^a-z])(ERROR|ERR)(:|[^a-z]|$)`)},
		{LevelWarn, regexp.MustCompile(`(?i)(^|[^a-z])(WARN(?:ING)?)(:|[^a-z]|$)`)},
		{LevelInfo, regexp.MustCompile(`(?i)(^|[^a-z])(INFO)(:|[^a-z]|$)`)},
		{LevelDebug, regexp.MustCompile(`(?i)(^|[^a-z])(DEBUG)(:|[^a-z]|$)`)},
		{LevelTrace, regexp.MustCompile(`(?i)(^|[^a-z])(TRACE)(:|[^a-z]|$)`)},
	}

	// kubectl severity prefix: E/W/I/F + date (e.g. "E0101 12:00:00.000")
	kubectlRE = regexp.MustCompile(`^([EWIFD])\d{4} `)

	// ISO/RFC timestamp at line start
	tsRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)`)
)

// parseLevel detects the log level from one log line.
func parseLevel(line string) string {
	// 1. kubectl-style prefix
	if m := kubectlRE.FindStringSubmatch(line); m != nil {
		switch m[1] {
		case "E", "F":
			return LevelError
		case "W":
			return LevelWarn
		case "I":
			return LevelInfo
		case "D":
			return LevelDebug
		}
	}
	// 2. JSON structured log
	if strings.HasPrefix(strings.TrimSpace(line), "{") {
		var obj map[string]interface{}
		if json.Unmarshal([]byte(line), &obj) == nil {
			for _, k := range []string{"level", "Level", "LEVEL", "severity", "lvl"} {
				if v, ok := obj[k].(string); ok {
					return normalizeLevel(v)
				}
			}
		}
	}
	// 3. Pattern match
	for _, p := range levelREs {
		if p.re.MatchString(line) {
			return p.level
		}
	}
	return LevelUnknown
}

func normalizeLevel(s string) string {
	u := strings.ToUpper(strings.TrimSpace(s))
	switch u {
	case "FATAL", "CRIT", "CRITICAL":
		return LevelFatal
	case "ERROR", "ERR":
		return LevelError
	case "WARN", "WARNING":
		return LevelWarn
	case "INFO", "INFORMATION":
		return LevelInfo
	case "DEBUG":
		return LevelDebug
	case "TRACE":
		return LevelTrace
	}
	return LevelUnknown
}

// parseTimestamp extracts the leading timestamp if present.
func parseTimestamp(line string) string {
	if m := tsRE.FindString(line); m != "" {
		return m
	}
	return ""
}

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
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
	tail, _ := strconv.ParseInt(r.URL.Query().Get("tail"), 10, 64)
	if tail <= 0 {
		tail = 300
	}
	container := r.URL.Query().Get("container")

	g := graph.Build(*res)
	node, findErr := findNode(g, resource)
	if findErr != nil {
		writeError(w, http.StatusNotFound, findErr.Error())
		return
	}

	// Collect pod names: if node IS a Pod, just that one.
	// Otherwise BFS forward to find all pods.
	var pods []struct{ ns, name string }
	if node.Kind == "Pod" {
		pods = append(pods, struct{ ns, name string }{node.Namespace, node.Name})
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
				pods = append(pods, struct{ ns, name string }{n.Namespace, n.Name})
				continue
			}
			queue = append(queue, g.Out(uid)...)
		}
	}

	if ns == "" && len(pods) > 0 {
		ns = pods[0].ns
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

	resp := LogsResponse{Context: ctxName}
	fetchCtx, cancel := context.WithTimeout(reqCtx, 30*time.Second)
	defer cancel()

	for _, pod := range pods {
		// Determine containers to fetch.
		var containers []string
		if container != "" {
			containers = []string{container}
		} else {
			// List containers from the pod spec.
			podObj, _ := client.CoreV1().Pods(pod.ns).Get(fetchCtx, pod.name, metav1.GetOptions{})
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
			opts := &corev1.PodLogOptions{TailLines: &tail}
			if cname != "" {
				opts.Container = cname
			}
			stream, streamErr := client.CoreV1().Pods(pod.ns).GetLogs(pod.name, opts).Stream(fetchCtx)
			result := PodLogResult{Pod: pod.name, Container: cname}
			if streamErr != nil {
				result.Error = streamErr.Error()
				resp.Pods = append(resp.Pods, result)
				continue
			}
			scanner := bufio.NewScanner(io.LimitReader(stream, 2*1024*1024)) // 2 MB max
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				raw := scanner.Text()
				entry := LogEntry{
					LineNum:   lineNum,
					Raw:       raw,
					Level:     parseLevel(raw),
					Timestamp: parseTimestamp(raw),
				}
				// For JSON logs, try to extract the message field.
				if strings.HasPrefix(strings.TrimSpace(raw), "{") {
					var obj map[string]interface{}
					if json.Unmarshal([]byte(raw), &obj) == nil {
						for _, mk := range []string{"msg", "message", "Message"} {
							if v, ok := obj[mk].(string); ok {
								entry.Message = v
								break
							}
						}
					}
				}
				result.Entries = append(result.Entries, entry)
			}
			if err := scanner.Err(); err != nil {
				result.Error = "read error: " + err.Error()
			}
			stream.Close()
			resp.Pods = append(resp.Pods, result)
		}
	}

	writeJSON(w, resp)
}
