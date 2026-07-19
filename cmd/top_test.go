package cmd

import (
	"strings"
	"testing"
)

func TestClassifyUsage(t *testing.T) {
	tests := []struct {
		name           string
		cpuLim, memLim int64
		cpuPct, memPct int
		wantContains   string
	}{
		{"over limit cpu", 100, 100, 95, 10, "over limit"},
		{"high mem", 100, 100, 10, 80, "high"},
		{"no limit", 0, 0, 0, 0, "no limit"},
		{"ok", 100, 100, 10, 20, "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, _ := classifyUsage(tt.cpuLim, tt.memLim, tt.cpuPct, tt.memPct)
			if !strings.Contains(status, tt.wantContains) {
				t.Errorf("classifyUsage status = %q, want containing %q", status, tt.wantContains)
			}
		})
	}
}

func TestFormatters(t *testing.T) {
	if got := fmtCPU(50); got != "50m" {
		t.Errorf("fmtCPU(50) = %q, want 50m", got)
	}
	if got := fmtCPU(1500); got != "1.50" {
		t.Errorf("fmtCPU(1500) = %q, want 1.50", got)
	}
	if got := fmtMem(64 * 1024 * 1024); got != "64Mi" {
		t.Errorf("fmtMem(64Mi) = %q, want 64Mi", got)
	}
	if got := fmtMem(2 * 1024 * 1024 * 1024); got != "2.0Gi" {
		t.Errorf("fmtMem(2Gi) = %q, want 2.0Gi", got)
	}
}
