#!/usr/bin/env bash
# dev.sh — Start KGraph in development mode with full hot-reload.
#
# Go changes  → air rebuilds + restarts the API server (http://localhost:7329)
# UI changes  → Vite HMR updates the browser instantly  (http://localhost:5173)
#
# Prerequisites (install once):
#   go install github.com/air-verse/air@latest
#   cd ui && npm install
#
# Usage:
#   ./dev.sh          # start both services
#   ./dev.sh --ui     # Vite only (if Go server already running)
#   ./dev.sh --api    # air only  (if Vite already running)

set -euo pipefail

MODE="${1:-both}"

# Check dependencies
if [[ "$MODE" != "--ui" ]] && ! command -v air &>/dev/null; then
  echo "❌  air not found. Install with: go install github.com/air-verse/air@latest"
  exit 1
fi

if [[ "$MODE" != "--api" ]] && [[ ! -d "ui/node_modules" ]]; then
  echo "📦  Installing UI dependencies..."
  (cd ui && npm install)
fi

cleanup() {
  echo ""
  echo "👋  Shutting down dev servers..."
  kill 0
}
trap cleanup SIGINT SIGTERM EXIT

case "$MODE" in
  --api)
    echo "⬡  KGraph API (air) → http://localhost:7329"
    air
    ;;
  --ui)
    echo "⬡  KGraph UI  (Vite) → http://localhost:5173"
    (cd ui && npm run dev)
    ;;
  both|*)
    echo ""
    echo "  ⬡  KGraph dev servers starting..."
    echo "  ┌─ API (air/Go)  → http://localhost:7329"
    echo "  └─ UI  (Vite)    → http://localhost:5173  ← open this"
    echo ""
    echo "  Go changes: auto-rebuild + restart"
    echo "  UI changes: instant HMR (no reload)"
    echo "  Press Ctrl+C to stop both."
    echo ""

    # Start Go API with air in background
    air &
    # Start Vite dev server in foreground
    (cd ui && npm run dev)
    ;;
esac
