#!/usr/bin/env bash
# Clone the open-design daemon next to wuphf and install its deps so
# the Designer / CMO / CEO agents' open-design MCP tools have a real
# backend to talk to.
#
# open-design (https://github.com/DYAI2025/open-design) ships its own
# pnpm workspace with a local Express daemon (default 127.0.0.1:7878).
# Once installed, start it manually:
#
#   cd ../open-design && pnpm tools-dev
#
# The MCP tools in wuphf check /api/health before each call and degrade
# cleanly when the daemon isn't running — so it's fine to keep it off
# until you actually need design escalation.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
target="$(dirname "$repo_root")/open-design"

if ! command -v pnpm >/dev/null 2>&1; then
  echo "pnpm not found on PATH." >&2
  echo "  install: corepack enable && corepack prepare pnpm@latest --activate" >&2
  exit 1
fi

if [[ -d "$target/.git" ]]; then
  echo "==> open-design already cloned at $target; pulling latest"
  git -C "$target" pull --ff-only
else
  echo "==> cloning open-design into $target"
  git clone --depth 1 https://github.com/DYAI2025/open-design.git "$target"
fi

echo "==> pnpm install (in $target)"
pnpm --dir "$target" install

cat <<EOF

open-design installed at $target.

Start the daemon:        cd $target && pnpm tools-dev
Override its URL:        export WUPHF_OPEN_DESIGN_URL=http://127.0.0.1:7878
Available agents:        designer, cmo, ceo (auto-discovered by slug)
EOF
