#!/usr/bin/env bash
# ==============================================================================
# sync-vps-hub.sh — Coleta métricas da VPS e de todos os bots e envia ao GitHub Pages
# ==============================================================================
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
node "$DIR/sync-vps-hub.js" --push
