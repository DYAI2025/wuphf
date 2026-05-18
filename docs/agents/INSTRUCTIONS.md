# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> `CLAUDE.md` is a symlink to `docs/agents/INSTRUCTIONS.md` — edit the target,
> not the link. Same applies to `AGENTS.md`.

## What Is WUPHF

WUPHF is a collaborative office for AI employees with a shared brain. It runs
a team of AI agents (CEO, PM, engineers, designer, etc.) that communicate via
channels, claim tasks, and ship work autonomously. It is **not a CRM** — it is
part of the Nex context-graph platform for AI agents.

Core runtime: Go binary + React web UI. Everything is local, single binary,
local SQLite/files — no SaaS backend.

## Commands

### Build

```bash
# First-time setup (installs deps, git hooks via lefthook)
./scripts/bootstrap.sh

# Build the Go binary (requires web assets)
cd web && bun install && bun run build && cd ..
go build -o wuphf ./cmd/wuphf

# Run
./wuphf
```

`cmd/` also contains auxiliary binaries — build them by path when needed:
`bench-slice-1`, `eval-prompts`, `review-bundle-handoff`, `wuphfbench`,
`wuphf-oc-probe`, `wuphf-seed`, `youtube-script-packet`.

### Workspace scripts (root `package.json`)

Root-level shortcuts that fan out to `packages/*` and `apps/*`:

```bash
bun run broker:test                  # packages/broker tests
bun run broker:typecheck
bun run broker:check:invariants
bun run desktop:dev                  # apps/desktop dev mode
bun run desktop:build
bun run desktop:test
bun run desktop:check:ipc-allowlist
bun run installer:build:dry-run
bun run installer:check:secrets
```

### Test

```bash
# Go — full suite with per-package fan-out and -race carve-out
bash scripts/test-go.sh

# Go — single package
bash scripts/test-go.sh ./internal/team

# Go — flake hunt (repeat N times)
COUNT=3 bash scripts/test-go.sh ./...

# Web — full suite (Vitest)
bash scripts/test-web.sh

# Web — single file
bash scripts/test-web.sh web/src/path/to/file.test.ts

# Package-specific (from package dir)
cd packages/protocol && bunx vitest run
cd packages/broker && bun run test
cd packages/llm-router && bunx vitest run
```

Do NOT use `bun test` inside `web/` — that invokes Bun's native runner instead
of Vitest. Do NOT use plain `go test -race ./...` — it triggers known flakes in
`internal/team`. Always use `scripts/test-go.sh`.

### Lint

```bash
# Go
gofmt -w <file>
go vet ./...
golangci-lint run ./...

# Web (from web/)
bunx biome check --write

# Secrets
bunx secretlint <files>
```

### Dev Server (Web UI)

```bash
cd web && bun run dev   # Vite dev server
bunx tsc --noEmit       # type-check only
```

Always use `bun` / `bunx` for JavaScript tooling in this repo.

## Architecture

```
human → Web UI / TUI → Broker (pub/sub + queue) ← optional integrations
                              │
                              ▼ push on message
              Per-agent headless runners (claude -p / codex)
                              │
                              ▼
                 Isolated git worktree per agent
```

### Three load-bearing choices

1. **Fresh session per turn** (`internal/team/headless_claude.go`) — every agent
   turn is `claude -p` from scratch. No `--resume`, no growing history. Prompt
   cache gives ~97% read hits.

2. **Per-agent scoped MCP** (`internal/teammcp/`) — each agent role gets exactly
   the tools it needs, nothing more. Smaller schema → cheaper turn → better
   cache alignment.

3. **Push-driven broker** (`internal/team/broker.go`) — agents sleep until
   pushed a message. No polling. Idle cost is zero.

### Key directories

| Path | Role |
|------|------|
| `cmd/wuphf/` | CLI entrypoint, slash commands, launcher |
| `internal/team/` | Broker, launcher, headless runners, worktree isolation, resume |
| `internal/teammcp/` | Per-agent MCP tool surface |
| `internal/agent/packs.go` | Team compositions (starter, founding-team, coding-team, etc.) |
| `web/` | React office UI (Vite, Biome, Vitest) |
| `packages/protocol/` | Wire-shape protocol library (TypeScript) |
| `packages/broker/` | TypeScript broker package |
| `packages/llm-router/` | LLM routing package |
| `apps/desktop/` | Electron desktop app |
| `apps/installer-stub/` | Installer/updater |
| `mcp/` | MCP servers for Nex context, human-in-the-loop approvals |

### Optional integrations

All load-time optional. Core is `broker + launcher + headless runners + worktrees`.

- **Nex** (`--no-nex` to disable) — context graph, email/CRM context
- **Telegram** — bidirectional bridge via `/connect`
- **Composio** (`--action provider`) — real-world actions
- **OpenClaw** (`--provider openclaw-http`) — OpenClaw Gateway bridge
- **Hermes** (`--provider hermes-agent`) — local Hermes gateway

## Local-First Models (Default)

The install-wide default LLM provider is **`ollama`**. First-run wuphf
needs no API keys — only a running `ollama serve` daemon with the three
models the agents are tuned for:

| Model            | Default agent fit                   | Pull size |
|------------------|-------------------------------------|-----------|
| `gemma4:e4b`     | reasoning, planning, conversation   | ~3 GB     |
| `qwen2.5-coder:7b` | code generation, refactoring      | ~5 GB     |
| `llama3.1:8b`    | long context, content, design copy  | ~5 GB     |

One-shot pull: `scripts/setup-local-models.sh` (add `--large` on 64 GB+
machines to also grab `gemma4:26b`, `qwen2.5-coder:32b`, `llama3.1:70b`).

### Per-agent model binding

A single `ollama serve` can host all three models simultaneously. Pin a
different model per agent slug to send the engineer to Qwen-coder and
the designer to Llama without spinning up separate provider Kinds:

```jsonc
// ~/.wuphf/config.json
{
  "llm_provider": "ollama",
  "provider_endpoints": {
    "ollama": {
      "model": "gemma4:e4b",
      "models_by_agent": {
        "eng": "qwen2.5-coder:14b",
        "fe":  "qwen2.5-coder:7b",
        "be":  "qwen2.5-coder:14b",
        "designer": "llama3.1:8b",
        "cmo": "llama3.1:8b",
        "ceo": "gemma4:e4b"
      }
    }
  }
}
```

Env equivalents take precedence: `WUPHF_OLLAMA_MODEL_<SLUG>` (slug
upper-cased, `-` → `_`). Cloud providers (claude-code, codex, opencode)
remain available via `--provider` override and `WUPHF_LLM_PROVIDER`.

## Open-Design Fallback (Designer / CMO / CEO)

For visual work that needs real pixels (decks, posters, landing pages,
dashboard mocks), the Designer, CMO, and CEO agents can call into a
local open-design daemon (`https://github.com/DYAI2025/open-design`).
Five MCP tools are exposed when the agent slug matches:

- `open_design_list_skills` (filter by scenario)
- `open_design_list_templates`
- `open_design_create_artifact`
- `open_design_list_artifacts`
- `open_design_refresh_artifact`

The agent decides — no router on the wuphf side. When the open-design
daemon isn't running, every tool returns a one-line "start the daemon"
message and the agent falls back to its local model.

Setup: `scripts/setup-open-design.sh` clones open-design next to wuphf
and runs `pnpm install`. Start the daemon manually with
`cd ../open-design && pnpm tools-dev`. Override its URL via
`WUPHF_OPEN_DESIGN_URL` (default `http://127.0.0.1:7878`).

## Reference Docs

Top-level docs worth reading before substantial work:

| File | When to read |
|------|--------------|
| `ARCHITECTURE.md` | system-wide design before structural changes |
| `DESIGN.md` / `DESIGN-NOTEBOOK.md` / `DESIGN-WIKI.md` | UX/product design decisions |
| `PLAN.md`, `TODOS.md`, `CHANGELOG.md`, `PHASE-0-LEDGER.md` | current roadmap + recent shipped work |
| `CONTRIBUTING.md` | contributor workflow details |
| `FORKING.md` | downstream/fork policy |
| `TESTING-WIKI.md` | flake handling, coverage conventions |
| `.rules` | Nex MCP tooling context (proactive `<nex-context>` blocks) |

## Git and PR Rules

- Never push directly to `main`. Branch + draft PR for all code changes.
- Conventional Commits (enforced by commitlint hook).
- Never use `--no-verify` to bypass hooks.
- Run the full relevant test suite before marking a PR ready.
- For PRs changing `web/`, capture screenshots via `web/e2e/screenshots/`.

### Git hooks (via lefthook)

**pre-commit** (parallel): gofmt, go vet, golangci-lint, biome (web + packages),
secretlint, no-secrets grep, merge-conflict check, 5MB file-size limit.

**pre-push** (serial): go-unit, web-unit, desktop-typecheck/test, smoke build,
protocol-demo, protocol-wire-contract, protocol-invariants, broker tests,
file-size budget (800 LOC warn / 1500 LOC fail).

## Quality Rules

- Do not suppress lint or type errors with ignore comments. Fix the code.
- Do not introduce explicit `any` in TypeScript.
- Do not commit secrets or inline API keys.
- File-size budget: warn at 800 LOC, fail at 1500 LOC (allowlist in
  `scripts/file-size-allowlist.txt`).

## Environment

| Variable | Default | Purpose |
|----------|---------|---------|
| `WUPHF_BASE_URL` | `https://app.nex.ai` | API base (staging: `https://app.staging.wuphf.ai`, local: `http://localhost:30000`) |

## Multi-Agent Review Process

For substantial changes (new packages, security boundaries, wire shapes):

1. Implement with local tests.
2. Multi-agent review with explicit lenses (perf, security, types, architecture).
3. Address CodeRabbit findings (re-reviews on every push).
4. Staff-engineer review pass.
5. Every PR comment gets a disposition: `FIXED` (+ commit ref), `SKIPPED` (+
   reason), or `DEFERRED` (+ issue link).

For routine bug fixes, this is overkill — use lighter review.

## Sub-Agent Dispatch

When delegating to sub-agents, the dispatch prompt MUST include:
1. Pointer to the relevant `AGENTS.md`
2. Hard rules pasted verbatim (sub-agents don't always read linked docs)
3. Explicit decision options for design ambiguity
4. Verification commands to run before commit
5. Per-finding disposition format (FIXED/SKIPPED/DEFERRED)
6. Scope boundary (files to touch vs. not touch)

## Protocol-Grade Packages

Packages defining a wire shape (`packages/protocol/`) must ship:
- A `scripts/demo.ts` exercising the public API end-to-end
- A cross-language reference verifier (`testdata/verifier-reference.go`)
- README updates in the same commit as wire-shape changes
