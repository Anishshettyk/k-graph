package api

// metrics_api.go — /api/metrics/snapshot and /api/events/cluster handlers.
//
// Metrics snapshot: combines live metrics-server usage with declared pod
// limits to produce enriched utilization percentages and bottleneck severity.
// No history is required — the client accumulates snapshots for sparklines.
//
// Event intelligence: aggregates cluster events, deduplicates by
// (reason + object), detects rapid-repeat patterns, flags cluster-wide issues.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/metrics"
)

// ─── /api/metrics/snapshot ──────────────────────────────────────────────────

type Severity string

const (
	SevOver     Severity = "over"     // >100 % of limit
	SevCritical Severity = "critical" // ≥90 %
	SevWarning  Severity = "warning"  // ≥75 %
	SevOK       Severity = "ok"
)

type PodMetricsInfo struct {
	Namespace     string   `json:"namespace"`
	Name          string   `json:"name"`
	CPUMilli      int64    `json:"cpuMilli"`
	CPULimitMilli int64    `json:"cpuLimitMilli"`
	CPUReqMilli   int64    `json:"cpuRequestMilli"`
	CPUPct        float64  `json:"cpuPct"`   // % of limit; -1 = no limit
	MemBytes      int64    `json:"memBytes"`
	MemLimitBytes int64    `json:"memLimitBytes"`
	MemReqBytes   int64    `json:"memRequestBytes"`
	MemPct        float64  `json:"memPct"` // % of limit; -1 = no limit
	NoLimits      bool     `json:"noLimits"`
	CPUSev        Severity `json:"cpuSev"`
	MemSev        Severity `json:"memSev"`
}

type Bottleneck struct {
	Namespace      string   `json:"namespace"`
	PodName        string   `json:"podName"`
	Resource       string   `json:"resource"`       // "cpu" | "memory"
	Severity       Severity `json:"severity"`
	UsagePct       float64  `json:"usagePct"`
	Recommendation string   `json:"recommendation"`
}

type MetricsSnapshotResponse struct {
	Context     string           `json:"context"`
	Namespace   string           `json:"namespace"`
	Timestamp   time.Time        `json:"timestamp"`
	Pods        []PodMetricsInfo `json:"pods"`
	Bottlenecks []Bottleneck     `json:"bottlenecks"`
	NoLimitPods []string         `json:"noLimitPods"`
	Available   bool             `json:"available"` // false when metrics-server absent
}

func (h *Handler) handleMetricsSnapshot(w http.ResponseWriter, r *http.Request) {
	reqCtx, ctxName, ns, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := MetricsSnapshotResponse{
		Context:   ctxName,
		Namespace: ns,
		Timestamp: time.Now().UTC(),
	}

	// Fetch live usage from metrics-server (best-effort — not all clusters have it).
	mc, mcErr := metrics.New(h.kubeconfig, ctxName)
	var usage []metrics.PodUsage
	if mcErr == nil {
		ctx2, cancel := context.WithTimeout(reqCtx, 10*time.Second)
		usage, _ = mc.Pods(ctx2)
		cancel()
	}
	resp.Available = len(usage) > 0

	// Build usage lookup: "namespace/name" → PodUsage.
	usageMap := make(map[string]metrics.PodUsage, len(usage))
	for _, u := range usage {
		usageMap[u.Namespace+"/"+u.Name] = u
	}

	// Enrich with limits from pod specs.
	for _, pod := range res.Pods {
		if ns != "" && pod.Namespace != ns {
			continue
		}
		var cpuLim, cpuReq, memLim, memReq int64
		for _, c := range pod.Spec.Containers {
			if v := c.Resources.Limits.Cpu(); v != nil {
				cpuLim += v.MilliValue()
			}
			if v := c.Resources.Requests.Cpu(); v != nil {
				cpuReq += v.MilliValue()
			}
			if v := c.Resources.Limits.Memory(); v != nil {
				memLim += v.Value()
			}
			if v := c.Resources.Requests.Memory(); v != nil {
				memReq += v.Value()
			}
		}
		u := usageMap[pod.Namespace+"/"+pod.Name]

		info := PodMetricsInfo{
			Namespace:     pod.Namespace,
			Name:          pod.Name,
			CPUMilli:      u.CPUMilli,
			CPULimitMilli: cpuLim,
			CPUReqMilli:   cpuReq,
			MemBytes:      u.MemBytes,
			MemLimitBytes: memLim,
			MemReqBytes:   memReq,
			NoLimits:      cpuLim == 0 && memLim == 0,
			CPUSev:        SevOK,
			MemSev:        SevOK,
		}
		if cpuLim > 0 {
			info.CPUPct = float64(u.CPUMilli) * 100 / float64(cpuLim)
			info.CPUSev = pctSeverity(info.CPUPct)
		} else {
			info.CPUPct = -1
		}
		if memLim > 0 {
			info.MemPct = float64(u.MemBytes) * 100 / float64(memLim)
			info.MemSev = pctSeverity(info.MemPct)
		} else {
			info.MemPct = -1
		}
		resp.Pods = append(resp.Pods, info)

		// Collect bottlenecks and no-limit pods.
		if info.NoLimits {
			resp.NoLimitPods = append(resp.NoLimitPods, pod.Namespace+"/"+pod.Name)
		}
		if info.CPUSev != SevOK {
			resp.Bottlenecks = append(resp.Bottlenecks, Bottleneck{
				Namespace:      pod.Namespace,
				PodName:        pod.Name,
				Resource:       "cpu",
				Severity:       info.CPUSev,
				UsagePct:       info.CPUPct,
				Recommendation: cpuRecommendation(info),
			})
		}
		if info.MemSev != SevOK {
			resp.Bottlenecks = append(resp.Bottlenecks, Bottleneck{
				Namespace:      pod.Namespace,
				PodName:        pod.Name,
				Resource:       "memory",
				Severity:       info.MemSev,
				UsagePct:       info.MemPct,
				Recommendation: memRecommendation(info),
			})
		}
	}

	// Sort bottlenecks: over first, then critical, then warning.
	sort.Slice(resp.Bottlenecks, func(i, j int) bool {
		return sevRank(resp.Bottlenecks[i].Severity) > sevRank(resp.Bottlenecks[j].Severity)
	})
	writeJSON(w, resp)
}

func pctSeverity(pct float64) Severity {
	switch {
	case pct > 100:
		return SevOver
	case pct >= 90:
		return SevCritical
	case pct >= 75:
		return SevWarning
	default:
		return SevOK
	}
}

func sevRank(s Severity) int {
	switch s {
	case SevOver:
		return 3
	case SevCritical:
		return 2
	case SevWarning:
		return 1
	default:
		return 0
	}
}

func cpuRecommendation(p PodMetricsInfo) string {
	if p.CPUPct > 100 {
		return fmt.Sprintf("Pod %s/%s is over its CPU limit — throttling in progress. Increase CPU limit or add replicas.", p.Namespace, p.Name)
	}
	if p.CPUPct >= 90 {
		return fmt.Sprintf("Pod %s/%s CPU at %.0f%% — consider increasing limit or adding replicas.", p.Namespace, p.Name, p.CPUPct)
	}
	return fmt.Sprintf("Pod %s/%s CPU at %.0f%% — monitor closely.", p.Namespace, p.Name, p.CPUPct)
}

func memRecommendation(p PodMetricsInfo) string {
	if p.MemPct > 100 {
		return fmt.Sprintf("Pod %s/%s exceeded memory limit — OOM kill imminent. Increase limit immediately.", p.Namespace, p.Name)
	}
	if p.MemPct >= 90 {
		return fmt.Sprintf("Pod %s/%s memory at %.0f%% — increase limit or check for memory leak.", p.Namespace, p.Name, p.MemPct)
	}
	return fmt.Sprintf("Pod %s/%s memory at %.0f%%.", p.Namespace, p.Name, p.MemPct)
}

// ─── /api/events/cluster ────────────────────────────────────────────────────

type EventSeverity string

const (
	EvtCritical   EventSeverity = "critical"    // pattern: many occurrences quickly
	EvtWarning    EventSeverity = "warning"      // repeated warning events
	EvtInfo       EventSeverity = "info"         // single normal event
	EvtCluster    EventSeverity = "cluster-wide" // same issue on multiple objects
)

type AggregatedEvent struct {
	Reason        string        `json:"reason"`
	Message       string        `json:"message"`
	Objects       []string      `json:"objects"`   // "Pod/payments-abc"
	Namespace     string        `json:"namespace"`
	Count         int32         `json:"count"`
	FirstSeen     time.Time     `json:"firstSeen"`
	LastSeen      time.Time     `json:"lastSeen"`
	Type          string        `json:"type"`      // Warning | Normal
	Severity      EventSeverity `json:"severity"`
	IsPattern     bool          `json:"isPattern"`
	IsClusterWide bool          `json:"isClusterWide"`
}

type ClusterEventsResponse struct {
	Context   string            `json:"context"`
	Namespace string            `json:"namespace"`
	Events    []AggregatedEvent `json:"events"`
	Patterns  int               `json:"patterns"` // count of high-severity patterns
}

func (h *Handler) handleClusterEvents(w http.ResponseWriter, r *http.Request) {
	reqCtx, ctxName, ns, _, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	restConfig, cfgErr := collector.RESTConfig(h.kubeconfig, ctxName)
	if cfgErr != nil {
		writeError(w, http.StatusInternalServerError, cfgErr.Error())
		return
	}
	client, clientErr := kubernetes.NewForConfig(restConfig)
	if clientErr != nil {
		writeError(w, http.StatusInternalServerError, clientErr.Error())
		return
	}

	listNS := ns
	if listNS == "" {
		listNS = metav1.NamespaceAll
	}

	fetchCtx, cancel := context.WithTimeout(reqCtx, 20*time.Second)
	defer cancel()

	list, listErr := client.CoreV1().Events(listNS).List(fetchCtx, metav1.ListOptions{})
	if listErr != nil {
		writeError(w, http.StatusInternalServerError, listErr.Error())
		return
	}

	// Aggregate events by (reason + objectKind + truncated message).
	type aggKey struct{ reason, kind, msgHash string }
	agg := make(map[aggKey]*AggregatedEvent)

	now := time.Now()
	for _, ev := range list.Items {
		ts := ev.LastTimestamp.Time
		if ts.IsZero() {
			ts = ev.EventTime.Time
		}
		if ts.IsZero() {
			ts = ev.CreationTimestamp.Time
		}
		// Only include events from the last 2 hours.
		if now.Sub(ts) > 2*time.Hour {
			continue
		}

		objKind := ev.InvolvedObject.Kind
		objName := ev.InvolvedObject.Name
		msgHash := truncate(ev.Message, 60)
		key := aggKey{ev.Reason, objKind, msgHash}

		if _, ok := agg[key]; !ok {
			agg[key] = &AggregatedEvent{
				Reason:    ev.Reason,
				Message:   ev.Message,
				Namespace: ev.Namespace,
				Type:      ev.Type,
				FirstSeen: ts,
				LastSeen:  ts,
			}
		}
		a := agg[key]
		a.Count += ev.Count
		objRef := objKind + "/" + objName
		if !containsStr(a.Objects, objRef) {
			a.Objects = append(a.Objects, objRef)
		}
		if ts.Before(a.FirstSeen) {
			a.FirstSeen = ts
		}
		if ts.After(a.LastSeen) {
			a.LastSeen = ts
		}
	}

	resp := ClusterEventsResponse{Context: ctxName, Namespace: ns}
	recentWindow := 10 * time.Minute

	for _, a := range agg {
		// Pattern: same reason repeating rapidly.
		a.IsPattern = a.Count >= 5 && now.Sub(a.FirstSeen) <= recentWindow
		// Cluster-wide: same issue on 3+ distinct objects.
		a.IsClusterWide = len(a.Objects) >= 3

		switch {
		case a.IsClusterWide && a.Type == corev1.EventTypeWarning:
			a.Severity = EvtCluster
		case a.IsPattern:
			a.Severity = EvtCritical
		case a.Type == corev1.EventTypeWarning:
			a.Severity = EvtWarning
		default:
			a.Severity = EvtInfo
		}
		resp.Events = append(resp.Events, *a)
	}

	// Sort: cluster-wide → critical → warning → info, then newest first.
	sort.Slice(resp.Events, func(i, j int) bool {
		ri, rj := evtSevRank(resp.Events[i].Severity), evtSevRank(resp.Events[j].Severity)
		if ri != rj {
			return ri > rj
		}
		return resp.Events[i].LastSeen.After(resp.Events[j].LastSeen)
	})

	for _, e := range resp.Events {
		if e.Severity == EvtCritical || e.Severity == EvtCluster {
			resp.Patterns++
		}
	}

	writeJSON(w, resp)
}

func evtSevRank(s EventSeverity) int {
	switch s {
	case EvtCluster:
		return 4
	case EvtCritical:
		return 3
	case EvtWarning:
		return 2
	default:
		return 1
	}
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// truncate shortens a string to max length, appending "…" if cut.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// Ensure metrics is imported (used above).
var _ = json.Marshal
