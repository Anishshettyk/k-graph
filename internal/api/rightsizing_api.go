package api

// rightsizing_api.go — /api/rightsizing: resource right-sizing recommendations.
//
// Cross-references actual CPU/memory usage (from metrics-server) against
// declared requests and limits to identify waste and under-provisioning.

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/anishetty/kgraph/internal/metrics"
)

type RSizingSeverity string

const (
	RSWaste     RSizingSeverity = "waste"      // using <20% of request → over-provisioned
	RSUnderReq  RSizingSeverity = "under-req"  // usage near or above request → may throttle
	RSOK        RSizingSeverity = "ok"
)

type ContainerSizing struct {
	Name          string          `json:"name"`
	Resource      string          `json:"resource"` // "cpu" | "memory"
	UsageMilli    int64           `json:"usageMilli,omitempty"`
	UsageBytes    int64           `json:"usageBytes,omitempty"`
	RequestMilli  int64           `json:"requestMilli,omitempty"`
	RequestBytes  int64           `json:"requestBytes,omitempty"`
	LimitMilli    int64           `json:"limitMilli,omitempty"`
	LimitBytes    int64           `json:"limitBytes,omitempty"`
	UsagePct      float64         `json:"usagePct"`  // % of request
	Severity      RSizingSeverity `json:"severity"`
	Recommendation string         `json:"recommendation"`
}

type PodSizing struct {
	Namespace  string            `json:"namespace"`
	Name       string            `json:"name"`
	CPU        ContainerSizing   `json:"cpu"`
	Memory     ContainerSizing   `json:"memory"`
	Workload   string            `json:"workload,omitempty"` // owning workload name
}

type RightsizingResponse struct {
	Context    string      `json:"context"`
	Namespace  string      `json:"namespace"`
	Timestamp  time.Time   `json:"timestamp"`
	Pods       []PodSizing `json:"pods"`
	Available  bool        `json:"available"`
	WasteCount int         `json:"wasteCount"`
	// Namespace breakdown
	NSBreakdown []NSResourceSummary `json:"namespaceBreakdown"`
}

type NSResourceSummary struct {
	Namespace    string  `json:"namespace"`
	CPUReqMilli  int64   `json:"cpuRequestMilli"`
	CPUUsedMilli int64   `json:"cpuUsedMilli"`
	MemReqBytes  int64   `json:"memRequestBytes"`
	MemUsedBytes int64   `json:"memUsedBytes"`
	PodCount     int     `json:"podCount"`
}

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

	// Fetch live usage
	mc, mcErr := metrics.New(h.kubeconfig, ctxName)
	var usage []metrics.PodUsage
	if mcErr == nil {
		ctx2, cancel := context.WithTimeout(reqCtx, 10*time.Second)
		usage, _ = mc.Pods(ctx2)
		cancel()
	}
	resp.Available = len(usage) > 0

	usageMap := map[string]metrics.PodUsage{}
	for _, u := range usage {
		usageMap[u.Namespace+"/"+u.Name] = u
	}

	// Per-namespace aggregation
	nsMap := map[string]*NSResourceSummary{}

	for _, pod := range res.Pods {
		if ns != "" && pod.Namespace != ns {
			continue
		}
		u := usageMap[pod.Namespace+"/"+pod.Name]

		// Aggregate requests across containers
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

		// Workload name from owner references
		workload := ""
		for _, ref := range pod.OwnerReferences {
			workload = ref.Kind + "/" + ref.Name
			break
		}

		cpuSizing := sizingFor("cpu", u.CPUMilli, cpuReq, cpuLim)
		memSizing := sizingFor("memory", u.MemBytes, memReq, memLim)

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

		// namespace aggregation
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

	// Sort pods: worst waste first, then under-provisioned, then ok
	sort.Slice(resp.Pods, func(i, j int) bool {
		ri := rsRank(resp.Pods[i].CPU.Severity) + rsRank(resp.Pods[i].Memory.Severity)
		rj := rsRank(resp.Pods[j].CPU.Severity) + rsRank(resp.Pods[j].Memory.Severity)
		if ri != rj {
			return ri > rj
		}
		return resp.Pods[i].Name < resp.Pods[j].Name
	})

	// Namespace breakdown sorted by CPU request desc
	for _, ns3 := range nsMap {
		resp.NSBreakdown = append(resp.NSBreakdown, *ns3)
	}
	sort.Slice(resp.NSBreakdown, func(i, j int) bool {
		return resp.NSBreakdown[i].CPUReqMilli > resp.NSBreakdown[j].CPUReqMilli
	})

	writeJSON(w, resp)
}

func sizingFor(resource string, used, request, limit int64) ContainerSizing {
	s := ContainerSizing{Resource: resource, Severity: RSOK}
	if resource == "cpu" {
		s.UsageMilli = used
		s.RequestMilli = request
		s.LimitMilli = limit
	} else {
		s.UsageBytes = used
		s.RequestBytes = request
		s.LimitBytes = limit
	}
	if request == 0 {
		return s // no request set — can't compute waste
	}
	s.UsagePct = float64(used) * 100 / float64(request)
	if s.UsagePct < 20 && used > 0 {
		s.Severity = RSWaste
		if resource == "cpu" {
			s.Recommendation = fmt.Sprintf("Using %dm of %dm request (%.0f%%) — consider lowering request to ~%dm",
				used, request, s.UsagePct, max64(used*3, 10))
		} else {
			s.Recommendation = fmt.Sprintf("Using %s of %s request (%.0f%%) — consider lowering request",
				fmtBytes(used), fmtBytes(request), s.UsagePct)
		}
	} else if request > 0 && used >= int64(float64(request)*0.85) {
		s.Severity = RSUnderReq
		if resource == "cpu" {
			s.Recommendation = fmt.Sprintf("Using %dm approaching %dm request — consider increasing request",
				used, request)
		} else {
			s.Recommendation = fmt.Sprintf("Using %s near %s request — consider increasing request",
				fmtBytes(used), fmtBytes(request))
		}
	}
	return s
}

func rsRank(s RSizingSeverity) int {
	switch s {
	case RSWaste:
		return 2
	case RSUnderReq:
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
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1fGi", float64(b)/1024/1024/1024)
	case b >= 1024*1024:
		return fmt.Sprintf("%.0fMi", float64(b)/1024/1024)
	default:
		return fmt.Sprintf("%.0fKi", float64(b)/1024)
	}
}
