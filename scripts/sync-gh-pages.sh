#!/usr/bin/env bash
# Script para sincronizar telemetria com GitHub Pages a cada X minutos (via cron)
set -e

PORT=${1:-3001}
DOCS_DIR="$(dirname "$0")/../docs"

mkdir -p "$DOCS_DIR"
curl -s "http://127.0.0.1:${PORT}/api/status" > "$DOCS_DIR/status.json"

if [ -d ".git" ]; then
    git add "$DOCS_DIR/status.json"
    git commit -m "chore: sync telemetry $(date +'%Y-%m-%d %H:%M:%S')" || true
    git push origin main || true
fi
