#!/usr/bin/env bash
# Pull the three local models wuphf's local-first defaults expect:
#
#   - gemma4:e4b           — reasoning, planning, conversation (~3 GB)
#   - qwen2.5-coder:7b     — code-generation (~5 GB)
#   - llama3.1:8b          — long-context, content, design copy (~5 GB)
#
# Total cold-pull: ~13 GB. Re-running this is idempotent — Ollama skips
# already-present manifests. Pass --large to additionally pull the
# bigger sizes for users with 64 GB+ machines.
#
# Requires: ollama on PATH (https://ollama.com/download). Start the
# daemon first with `ollama serve` (or `brew services start ollama` on
# macOS).
set -euo pipefail

if ! command -v ollama >/dev/null 2>&1; then
  echo "ollama not found on PATH." >&2
  echo "  install: https://ollama.com/download" >&2
  exit 1
fi

models=(
  "gemma4:e4b"
  "qwen2.5-coder:7b"
  "llama3.1:8b"
)

if [[ "${1:-}" == "--large" ]]; then
  models+=("gemma4:26b" "qwen2.5-coder:32b" "llama3.1:70b")
fi

for m in "${models[@]}"; do
  echo "==> ollama pull $m"
  ollama pull "$m"
done

cat <<EOF

Pulled $(printf '%s, ' "${models[@]}" | sed 's/, $//').

Next steps:
  1. Make sure the ollama daemon is running: ollama serve  (or via Homebrew services)
  2. Run wuphf — it will default to provider=ollama with gemma4:e4b.
  3. For per-agent model binding, see docs/agents/INSTRUCTIONS.md
     ("Per-agent local model binding").
EOF
