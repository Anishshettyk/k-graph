package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Handler is the single HTTP handler for the KGraph JSON API.
// Every endpoint calls the same internal packages as the CLI commands.
type Handler struct {
	kubeconfig string
}

// New returns an API handler backed by the given kubeconfig path.
func New(kubeconfig string) *Handler {
	return &Handler{kubeconfig: kubeconfig}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Allow the Vite dev server (localhost:5173) during local development.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	seg := strings.TrimPrefix(r.URL.Path, "/api/")
	seg = strings.SplitN(seg, "/", 2)[0]

	switch seg {
	case "contexts":
		h.handleContexts(w, r)
	case "graph":
		h.handleGraph(w, r)
	case "deps":
		h.handleDeps(w, r)
	case "impact":
		h.handleImpact(w, r)
	case "why":
		h.handleWhy(w, r)
	case "doctor":
		h.handleDoctor(w, r)
	case "orphan":
		h.handleOrphan(w, r)
	case "rbac":
		h.handleRBAC(w, r)
	case "network":
		h.handleNetwork(w, r)
	case "can-reach":
		h.handleCanReach(w, r)
	case "yaml":
		h.handleYAML(w, r)
	case "logs":
		h.handleLogs(w, r)
	case "metrics":
		h.handleMetricsSnapshot(w, r)
	case "rightsizing":
		h.handleRightsizing(w, r)
	case "images":
		h.handleImages(w, r)
	case "certs":
		h.handleCerts(w, r)
	case "history":
		h.handleHistory(w, r)
	case "events":
		// /api/events (resource-scoped) vs /api/events/cluster
		if strings.HasPrefix(strings.TrimPrefix(r.URL.Path, "/api/"), "events/cluster") {
			h.handleClusterEvents(w, r)
		} else {
			h.handleEvents(w, r)
		}
	default:
		writeError(w, http.StatusNotFound, "unknown endpoint: /api/"+seg)
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
