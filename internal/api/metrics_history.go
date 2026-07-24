package api

// metrics_history.go — rolling in-memory metrics history for rightsizing.
//
// metrics-server only provides the current reading. A single point is
// insufficient for rightsizing: a pod may be idle right now but normally
// runs at 80%, or may spike briefly and appear over-limit.
//
// This accumulates the last maxSamples readings per (context, pod) and
// computes min/max/avg/p95 so recommendations are based on actual usage
// patterns over time rather than a single snapshot.

import (
	"sort"
	"sync"
	"time"
)

const maxSamples = 30 // 30 samples × 30s ≈ 15 minutes of history

// PodMetricsSample is one reading from metrics-server.
type PodMetricsSample struct {
	At       time.Time
	CPUMilli int64
	MemBytes int64
}

// MetricsStats is computed from a slice of samples.
type MetricsStats struct {
	Min    int64
	Max    int64
	Avg    int64
	P95    int64
	Count  int
	Oldest time.Time
}

// Empty is true when there are no samples.
func (s MetricsStats) Empty() bool { return s.Count == 0 }

// metricsHistory is a bounded per-pod rolling buffer.
// Key: "context/namespace/name"
type metricsHistory struct {
	mu      sync.Mutex
	entries map[string][]PodMetricsSample
}

func newMetricsHistory() *metricsHistory {
	return &metricsHistory{entries: make(map[string][]PodMetricsSample)}
}

func (h *metricsHistory) append(ctxName, namespace, name string, sample PodMetricsSample) {
	key := ctxName + "/" + namespace + "/" + name
	h.mu.Lock()
	defer h.mu.Unlock()
	buf := h.entries[key]
	buf = append(buf, sample)
	// keep only the most recent maxSamples
	if len(buf) > maxSamples {
		buf = buf[len(buf)-maxSamples:]
	}
	h.entries[key] = buf
}

func (h *metricsHistory) stats(ctxName, namespace, name string) (cpu MetricsStats, mem MetricsStats) {
	key := ctxName + "/" + namespace + "/" + name
	h.mu.Lock()
	buf := make([]PodMetricsSample, len(h.entries[key]))
	copy(buf, h.entries[key])
	h.mu.Unlock()

	if len(buf) == 0 {
		return
	}
	cpu.Count = len(buf)
	mem.Count = len(buf)
	cpu.Oldest = buf[0].At
	mem.Oldest = buf[0].At

	cpuVals := make([]int64, len(buf))
	memVals := make([]int64, len(buf))
	var cpuSum, memSum int64

	cpu.Min, mem.Min = buf[0].CPUMilli, buf[0].MemBytes
	cpu.Max, mem.Max = buf[0].CPUMilli, buf[0].MemBytes

	for i, s := range buf {
		cpuVals[i] = s.CPUMilli
		memVals[i] = s.MemBytes
		cpuSum += s.CPUMilli
		memSum += s.MemBytes
		if s.CPUMilli < cpu.Min {
			cpu.Min = s.CPUMilli
		}
		if s.CPUMilli > cpu.Max {
			cpu.Max = s.CPUMilli
		}
		if s.MemBytes < mem.Min {
			mem.Min = s.MemBytes
		}
		if s.MemBytes > mem.Max {
			mem.Max = s.MemBytes
		}
	}
	n := int64(len(buf))
	cpu.Avg = cpuSum / n
	mem.Avg = memSum / n
	cpu.P95 = p95(cpuVals)
	mem.P95 = p95(memVals)
	return
}

func p95(vals []int64) int64 {
	if len(vals) == 0 {
		return 0
	}
	s := make([]int64, len(vals))
	copy(s, vals)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	idx := int(float64(len(s)) * 0.95)
	if idx >= len(s) {
		idx = len(s) - 1
	}
	return s[idx]
}
