package api

// rightsizing_api.go — /api/rightsizing: range-based resource right-sizing.
//
// A single metrics snapshot is insufficient for rightsizing recommendations.
// This handler accumulates a rolling window of up to 30 samples (~15 minutes
// at 30s poll intervals) and computes min/max/avg/p95 before deciding
// whether a pod is over- or under-provisioned.
//
// Confidence levels:
//   LOW    (<  3 samples)  — not enough data; show values but hold recommendations
//   MEDIUM ( 3–9 samples)  — short window; recommendations are tentative
//   HIGH   (≥ 10 samples)  — solid trend; recommendations are reliable

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/anishetty/kgraph/internal/metrics"
)

// ─── Types ───────────────────────────────────────────────────────────────────

type RSizingSeverity string

const (
	RSWaste    RSizingSeverity = "waste"     // consistently under-utilised
	RSUnderReq RSizingSeverity = "under-req" // p95 near or above request
	RSSpiky    RSizingSeverity = "spiky"     // max/avg > 3 — needs burst headroom
	RSOK       RSizingSeverity = "ok"
)

type RangeStats struct {
	Min     int64   `json:"min"`
	Max     int64   `json:"max"`
	Avg     int64   `json:"avg"`
	P95     int64   `json:"p95"`
	Current int64   `json:"current"`
	Samples int     `json:"samples"`
	WindowM float64 `json:"windowMinutes"` // how long the window covers
}

type ContainerSizing struct {
	Resource       string          `json:"resource"` // "cpu" | "memory"
	Request        int64           `json:"request"`  // millicores or bytes
	Limit          int64           `json:"limit"`
	Stats          RangeStats      `json:"stats"`
	P95Pct         float64         `json:"p95Pct"`   // p95 as % of request
	MaxPct         float64         `json:"maxPct"`   // max as % of request
	Severity       RSizingSeverity `json:"severity"`
	Recommendation string          `json:"recommendation"`
	Confidence     string          `json:"confidence"` // low / medium / high
}

type PodSizing struct {
	Namespace string          `json:"namespace"`
	Name      string          `json:"name"`
	CPU       ContainerSizing `json:"cpu"`
	Memory    ContainerSizing `json:"memory"`
	Workload  string          `json:"workload,omitempty"`
}

type NSResourceSummary struct {
	Namespace    string  `json:"namespace"`
	CPUReqMilli  int64   `json:"cpuRequestMilli"`
	CPUUsedMilli int64   `json:"cpuUsedMilli"`
	MemReqBytes  int64   `json:"memRequestBytes"`
	MemUsedBytes int64   `json:"memUsedBytes"`
	PodCount     int     `json:"podCount"`
}

type RightsizingResponse struct {
	Context       string              `json:"context"`
	Namespace     string              `json:"namespace"`
	Timestamp     time.Time           `json:"timestamp"`
	Pods          []PodSizing         `json:"pods"`
	Available     bool                `json:"available"`
	WasteCount    int                 `json:"wasteCount"`
	NSBreakdown   []NSResourceSummary `json:"namespaceBreakdown"`
	// How many samples the server has collected for most pods in this window.
	TypicalSamples int    `json:"typicalSamples"`
	WindowMinutes  float64 `json:"windowMinutes"`
}

// ─── Handler ──────────────────────────────────────────────────────────────────

func (h *Handler) handleRightsizing(w http.ResponseWriter, r *http.Request) {
	reqCtx, ctxName, ns, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := RightsizingResponse{
		Context:   ctxName,
		Namespace: ns,
		Timestamp: time.Now().UTC(),
		Pods:      []PodSizing{},
	}

	// Fetch current metrics and append to rolling history.
	mc, mcErr := metrics.New(h.kubeconfig, ctxName)
	var usage []metrics.PodUsage
	if mcErr == nil {
		ctx2, cancel := context.WithTimeout(reqCtx, 10*time.Second)
		usage, _ = mc.Pods(ctx2)
		cancel()
	}
	resp.Available = len(usage) > 0

	// Append to history regardless of whether we got readings.
	now := time.Now()
	for _, u := range usage {
		h.metricsHistory.append(ctxName, u.Namespace, u.Name, PodMetricsSample{
			At:       now,
			CPUMilli: u.CPUMilli,
			MemBytes: u.MemBytes,
		})
	}

	// Also append zero readings for pods that are running but returned no metrics
	// (so history records their absence — helps detect if metrics are unavailable).

	usageMap := map[string]metrics.PodUsage{}
	for _, u := range usage {
		usageMap[u.Namespace+"/"+u.Name] = u
	}

	nsMap := map[string]*NSResourceSummary{}
	sampleCounts := []int{}

	for _, pod := range res.Pods {
		if ns != "" && pod.Namespace != ns {
			continue
		}
		u := usageMap[pod.Namespace+"/"+pod.Name]

		var cpuReq, memReq, cpuLim, memLim int64
		for _, c := range pod.Spec.Containers {
			if v := c.Resources.Requests.Cpu(); v != nil {
				cpuReq += v.MilliValue()
			}
			if v := c.Resources.Requests.Memory(); v != nil {
				memReq += v.Value()
			}
			if v := c.Resources.Limits.Cpu(); v != nil {
				cpuLim += v.MilliValue()
			}
			if v := c.Resources.Limits.Memory(); v != nil {
				memLim += v.Value()
			}
		}

		workload := ""
		for _, ref := range pod.OwnerReferences {
			workload = ref.Kind + "/" + ref.Name
			break
		}

		// Get historical stats
		cpuHist, memHist := h.metricsHistory.stats(ctxName, pod.Namespace, pod.Name)
		if cpuHist.Count == 0 && u.CPUMilli > 0 {
			// First reading — use current as a single sample
			cpuHist = MetricsStats{Min: u.CPUMilli, Max: u.CPUMilli, Avg: u.CPUMilli, P95: u.CPUMilli, Count: 1}
			memHist = MetricsStats{Min: u.MemBytes, Max: u.MemBytes, Avg: u.MemBytes, P95: u.MemBytes, Count: 1}
		}

		sampleCounts = append(sampleCounts, cpuHist.Count)

		windowMin := 0.0
		if cpuHist.Count > 1 && !cpuHist.Oldest.IsZero() {
			windowMin = time.Since(cpuHist.Oldest).Minutes()
		}

		cpuSizing := buildSizing("cpu", cpuReq, cpuLim, cpuHist)
		memSizing := buildSizing("memory", memReq, memLim, memHist)

		// Fill in current values from the latest reading
		cpuSizing.Stats.Current = u.CPUMilli
		memSizing.Stats.Current = u.MemBytes
		cpuSizing.Stats.WindowM = windowMin
		memSizing.Stats.WindowM = windowMin

		ps := PodSizing{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			CPU:       cpuSizing,
			Memory:    memSizing,
			Workload:  workload,
		}
		resp.Pods = append(resp.Pods, ps)

		if cpuSizing.Severity == RSWaste || memSizing.Severity == RSWaste {
			resp.WasteCount++
		}

		if _, ok := nsMap[pod.Namespace]; !ok {
			nsMap[pod.Namespace] = &NSResourceSummary{Namespace: pod.Namespace}
		}
		ns2 := nsMap[pod.Namespace]
		ns2.CPUReqMilli += cpuReq
		ns2.CPUUsedMilli += u.CPUMilli
		ns2.MemReqBytes += memReq
		ns2.MemUsedBytes += u.MemBytes
		ns2.PodCount++
	}

	// Sort: worst first
	sort.Slice(resp.Pods, func(i, j int) bool {
		ri := rsRank(resp.Pods[i].CPU.Severity) + rsRank(resp.Pods[i].Memory.Severity)
		rj := rsRank(resp.Pods[j].CPU.Severity) + rsRank(resp.Pods[j].Memory.Severity)
		if ri != rj {
			return ri > rj
		}
		return resp.Pods[i].Name < resp.Pods[j].Name
	})

	for _, ns3 := range nsMap {
		resp.NSBreakdown = append(resp.NSBreakdown, *ns3)
	}
	sort.Slice(resp.NSBreakdown, func(i, j int) bool {
		return resp.NSBreakdown[i].CPUReqMilli > resp.NSBreakdown[j].CPUReqMilli
	})

	// Typical sample count for reporting
	if len(sampleCounts) > 0 {
		sort.Ints(sampleCounts)
		resp.TypicalSamples = sampleCounts[len(sampleCounts)/2] // median
		if resp.TypicalSamples > 0 {
			// Estimate window from oldest sample in history (approximate)
			resp.WindowMinutes = float64(resp.TypicalSamples-1) * 0.5 // 30s intervals
		}
	}

	writeJSON(w, resp)
}

// ─── Analysis logic ───────────────────────────────────────────────────────────

func buildSizing(resource string, request, limit int64, hist MetricsStats) ContainerSizing {
	s := ContainerSizing{
		Resource:   resource,
		Request:    request,
		Limit:      limit,
		Severity:   RSOK,
		Confidence: confidence(hist.Count),
	}
	s.Stats = RangeStats{
		Min:     hist.Min,
		Max:     hist.Max,
		Avg:     hist.Avg,
		P95:     hist.P95,
		Current: hist.Min, // will be overwritten by caller with live reading
		Samples: hist.Count,
	}

	if request == 0 || hist.Count == 0 {
		return s // no request set or no data — can't compute
	}

	s.P95Pct = float64(hist.P95) * 100 / float64(request)
	s.MaxPct = float64(hist.Max) * 100 / float64(request)
	isSpiky := hist.Max > 0 && hist.Avg > 0 && float64(hist.Max)/float64(hist.Avg) > 3.0

	// Only make strong recommendations when we have enough data
	minSamplesForRec := 3

	switch {
	case hist.Count < minSamplesForRec:
		// Not enough data — just report, no severity
		s.Recommendation = fmt.Sprintf(
			"Collecting data — %d of %d samples gathered (check again in a few minutes)",
			hist.Count, minSamplesForRec)

	case isSpiky:
		// High variability — don't recommend reducing, flag for review
		s.Severity = RSSpiky
		if resource == "cpu" {
			s.Recommendation = fmt.Sprintf(
				"Spiky usage (max=%dm, avg=%dm, ratio=%.1fx) — keep request≥%dm to handle bursts; consider setting a higher limit",
				hist.Max, hist.Avg, float64(hist.Max)/float64(hist.Avg), hist.P95*2)
		} else {
			s.Recommendation = fmt.Sprintf(
				"Spiky usage (max=%s, avg=%s) — keep adequate request to handle bursts",
				fmtBytes(hist.Max), fmtBytes(hist.Avg))
		}

	case s.P95Pct < 20:
		// Consistently under-utilised — safe to reduce
		s.Severity = RSWaste
		headroom := 2 // 2× p95 for headroom
		if resource == "cpu" {
			suggested := max64(hist.P95*int64(headroom), 10)
			s.Recommendation = fmt.Sprintf(
				"Consistent waste: p95=%dm (%.0f%% of %dm request) — safe to set request≈%dm",
				hist.P95, s.P95Pct, request, suggested)
		} else {
			suggested := max64(hist.P95*int64(headroom), 4*1024*1024)
			s.Recommendation = fmt.Sprintf(
				"Consistent waste: p95=%s (%.0f%% of %s request) — safe to set request≈%s",
				fmtBytes(hist.P95), s.P95Pct, fmtBytes(request), fmtBytes(suggested))
		}

	case s.P95Pct >= 85:
		// p95 is near or above request — risk of throttling/OOM
		s.Severity = RSUnderReq
		if resource == "cpu" {
			suggested := hist.P95 * 120 / 100 // 20% headroom above p95
			s.Recommendation = fmt.Sprintf(
				"Under-provisioned: p95=%dm is %.0f%% of %dm request — CPU throttling likely; suggest request≈%dm",
				hist.P95, s.P95Pct, request, suggested)
		} else {
			suggested := hist.P95 * 120 / 100
			s.Recommendation = fmt.Sprintf(
				"Under-provisioned: p95=%s is %.0f%% of %s request — OOM risk; suggest request≈%s",
				fmtBytes(hist.P95), s.P95Pct, fmtBytes(request), fmtBytes(suggested))
		}

	default:
		if resource == "cpu" {
			s.Recommendation = fmt.Sprintf(
				"Well-sized: p95=%dm (%.0f%% of %dm request), max=%dm (%.0f%%) — no action needed",
				hist.P95, s.P95Pct, request, hist.Max, s.MaxPct)
		} else {
			s.Recommendation = fmt.Sprintf(
				"Well-sized: p95=%s (%.0f%% of %s request), max=%s (%.0f%%) — no action needed",
				fmtBytes(hist.P95), s.P95Pct, fmtBytes(request), fmtBytes(hist.Max), s.MaxPct)
		}
	}

	return s
}

func confidence(samples int) string {
	switch {
	case samples >= 10:
		return "high"
	case samples >= 3:
		return "medium"
	default:
		return "low"
	}
}

func rsRank(s RSizingSeverity) int {
	switch s {
	case RSWaste:
		return 3
	case RSUnderReq:
		return 2
	case RSSpiky:
		return 1
	default:
		return 0
	}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func fmtBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fGi", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0fMi", float64(b)/float64(1<<20))
	default:
		return fmt.Sprintf("%.0fKi", float64(b)/1024)
	}
}
