#!/usr/bin/env bash
# Record the graph tour with VHS (reliable TUI drive) + GitHub Light.
# Canvas: 1400×880 — same ballpark as the restyled take.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export PATH="$ROOT:$PATH"
# Agent shells often set NO_COLOR=1; that makes creel paint monochrome.
unset NO_COLOR
export CLICOLOR_FORCE=1
export COLORTERM=truecolor

command -v creel >/dev/null || go build -o "$ROOT/creel" ./cmd/creel
command -v vhs >/dev/null || { echo "vhs not found (brew install vhs)"; exit 1; }
[[ -f demo/creel-demo.db ]] || sqlite3 demo/creel-demo.db < demo/schema.sql

CFG=/tmp/creel-vhs-config
rm -rf "$CFG"
mkdir -p "$CFG/creel"
cat > "$CFG/creel/config.yaml" <<'EOF'
settings:
  theme: git-hub-light-default
  transparent_background: true
  status_hints: false
EOF

vhs demo/graph-tour.tape
ls -lh docs/images/demo-graph.gif
echo "Regenerated docs/images/demo-graph.gif"
