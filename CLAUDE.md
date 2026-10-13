# CLAUDE.md

## What we're building

A tool that makes local enterprise AI simple. It scans the compute an enterprise already owns, matches the best open-weight models to it, deploys them in one click, and serves them through one private OpenAI-compatible API, with hot swap and management. Everything runs inside the customer's own environment.

The full product has five phases. **We are building Phase 1 only: Scan and Report.** The spec is in `SPEC.md`. Read it before starting any task.

## Product rules (do not break these)

- **Read-only scanning.** Nothing in Phase 1 installs, starts or changes anything on customer infrastructure. Connectors use read-only permissions.
- **Data never leaves the customer's environment.** No telemetry, analytics or calls to outside services from the server or agent.
- **No jargon in the UI.** Say "GPU memory", not "VRAM". Unknown values say "unknown".
- **The API is the product.** Never add a chat interface.
- **Assemble, don't reinvent.** Prefer well-maintained open-source libraries over writing our own.
- Stay inside Phase 1 scope. If a task seems to need something from a later phase, stop and ask.

## Stack

- Go (latest stable) for server, agent and connectors
- TypeScript + React + Vite for `/web`
- Postgres with SQL migrations in `/internal/store/migrations`
- Key libraries: `github.com/NVIDIA/go-nvml`, `github.com/shirou/gopsutil`, `k8s.io/client-go`, `github.com/aws/aws-sdk-go-v2`
- Ask before adding a dependency not listed here or in `SPEC.md`.

## Commands

Set these up early and keep them working:

- `make dev` — run everything locally via Docker Compose
- `make test` — all Go and web tests
- `make lint` — `golangci-lint` and the web linter
- `make agent-fake` — run an agent in fake mode from a fixture

## How to work

- Build in small milestones and run `make test` after each one.
- Write tests alongside code: table-driven tests for merge, idle calculation and connector parsing.
- Use the fake agent and fixtures in `/testdata/fixtures` to test without GPU hardware.
- Never log secrets, tokens or credentials.
- Errors reading one device or one source must never crash the agent or a scan; log and continue.
- Keep `README.md` install steps current as features land.
