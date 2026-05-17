# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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
