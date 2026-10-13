# Phase 1 Spec: Scan and Report

## Goal

Build the first shippable piece of the product: find every machine and GPU an enterprise owns, learn when each one is idle, and show it all in one plain-English scan report.

This is the free product and the sales hook. It must be read-only: it never installs, starts or changes anything on a customer machine other than the agent the customer installs themselves.

## Scope

**In Phase 1**

- Node agent (Linux first, then macOS and Windows)
- Kubernetes connector
- AWS connector
- Core server with REST API and Postgres storage
- Web app with Setup, Report, Machines and Machine Detail pages
- Fake mode for developing without GPU hardware
- Docker Compose for running everything locally

**Not in Phase 1** (later phases, do not build now)

- Model catalogue, fit engine, recommendations
- Deploying models, inference engines, gateway, API keys
- Hot swap, SSO, audit log, Helm chart
- VMware, Proxmox, Azure, Google Cloud connectors

## Phase 1 decisions

- Language: Go for server, agent and connectors. TypeScript + React + Vite for the web app.
- Utilisation samples are stored in Postgres in Phase 1 (14-day retention) to keep the install simple. Prometheus comes later.
- Agent authentication: TLS plus a per-agent token issued at enrolment. Mutual TLS comes in a later phase.
- UI copy says "GPU memory", not "VRAM". No jargon on screen.

## Repository layout

```
/cmd/agent            agent binary
/cmd/server           server binary
/internal/agent       hardware collection (gpu_nvidia, gpu_amd, system, fake)
/internal/connectors  kubernetes/, aws/ (shared Connector interface)
/internal/inventory   machine merge, de-duplication, idle calculation
/internal/api         HTTP handlers and request validation
/internal/store       Postgres access and migrations
/web                  React + TypeScript app
/testdata/fixtures    fake machines and samples
/deploy               docker-compose.yml
```

## Components

### Node agent

- Single static Go binary. Runs as a service.
- On start and every 6 hours: sends a full **hardware report**.
- Every 60 seconds: sends a **utilisation sample**.
- Connects outbound only to the server over HTTPS. No listening ports.
- Enrolment: `agent enroll --server https://host --token <enrolment-token>` exchanges a one-time enrolment token for a per-agent token, stored locally with file permissions 0600.
- Hardware collection:
  - NVIDIA: `github.com/NVIDIA/go-nvml` (GPU name, memory total/used, utilisation, driver version, UUID). If NVML is missing, report no NVIDIA GPUs and continue.
  - AMD: run `amd-smi` with JSON output if present; parse name, memory, utilisation. If missing, continue.
  - Apple silicon: report chip name and unified memory size (GPU memory = unified memory).
  - CPU, RAM, disk, OS, hostname: `github.com/shirou/gopsutil`.
  - Stable machine ID: system UUID where available (`/sys/class/dmi/id/product_uuid` on Linux), else a generated ID stored on first run.
- Fake mode: `agent run --fake testdata/fixtures/<file>.json` sends the fixture's hardware and generated utilisation (busy 9am–6pm weekdays, idle otherwise) so the full pipeline can be tested on a laptop with no GPU.
- Any collection error is logged and skipped; the agent never crashes because one device can't be read.

### Kubernetes connector

- Uses `k8s.io/client-go` with a kubeconfig or in-cluster service account.
- Read-only RBAC: `get`, `list` on `nodes` only. Ship the ClusterRole YAML in `/deploy`.
- For each node: name, `providerID`, CPU and memory capacity, and GPU count from `nvidia.com/gpu` or `amd.com/gpu` capacity.
- GPU model and memory from NVIDIA GPU Feature Discovery labels when present: `nvidia.com/gpu.product`, `nvidia.com/gpu.memory` (MiB), `nvidia.com/gpu.count`. If labels are missing, record GPU count only and mark model as "unknown".
- Utilisation is "unknown" for Kubernetes nodes unless an agent also runs on that node (merge handles this).

### AWS connector

- Uses `github.com/aws/aws-sdk-go-v2`.
- Read-only IAM policy (ship it in `/deploy`): `ec2:DescribeRegions`, `ec2:DescribeInstances`, `ec2:DescribeInstanceTypes`.
- Scans all enabled regions, or a configured list.
- For each running or stopped instance: instance ID, type, region, state, name tag.
- From `DescribeInstanceTypes`: vCPU count, memory, and `GpuInfo` (GPU name, manufacturer, count, memory per GPU in MiB). Cache instance-type lookups.
- Utilisation is "unknown" unless an agent runs on the instance.

### Connector interface

All connectors implement one interface so new ones (VMware, Proxmox, Azure, GCP) slot in later:

```go
type Connector interface {
    Kind() string                                  // "kubernetes", "aws"
    Scan(ctx context.Context) ([]DiscoveredMachine, error)
}
```

### Inventory and merge

- Each machine is stored once, even if seen by several sources.
- Match order: AWS instance ID ↔ Kubernetes `providerID` ↔ agent-reported cloud instance ID (agent reads it from the instance metadata service when on AWS) ↔ system UUID ↔ hostname.
- Agent data wins for hardware detail; connector data fills gaps.
- Every machine records its sources (e.g. `["aws", "kubernetes", "agent"]`).

### Idle calculation

- A GPU sample counts as **idle** when GPU utilisation < 10% and GPU memory used < 10%.
- A machine with no GPU is idle when CPU utilisation < 10%.
- Per machine, over the last 7 days: percentage of time idle, and an "idle hours" pattern (for each hour of the week, idle in at least 80% of samples).
- Needs at least 24 hours of samples before showing a percentage; before that, show "still learning".

### Server API (JSON over HTTPS)

| Method | Path | Purpose |
| --- | --- | --- |
| POST | `/api/v1/agents/enroll` | Exchange enrolment token for agent token |
| POST | `/api/v1/agents/report` | Full hardware report from an agent |
| POST | `/api/v1/agents/samples` | Batch of utilisation samples |
| POST | `/api/v1/connectors` | Add a connector (kind + credentials) |
| POST | `/api/v1/connectors/{id}/scan` | Run a scan now |
| GET | `/api/v1/connectors` | List connectors and last scan status |
| GET | `/api/v1/machines` | List machines with GPUs and idle % |
| GET | `/api/v1/machines/{id}` | One machine with GPUs, sources, idle pattern |
| GET | `/api/v1/report` | Scan report totals and highlights |
| POST | `/api/v1/enrolment-tokens` | Create an enrolment token for installing agents |

- Connectors also scan automatically every 6 hours.
- Connector credentials are encrypted at rest (AES-GCM, key from an environment variable).
- Phase 1 web login: single admin account with a password set at first run. SSO comes later.

### Data shapes

Agent hardware report:

```json
{
  "machine_id": "string",
  "hostname": "string",
  "os": "linux",
  "cloud": { "provider": "aws", "instance_id": "i-0abc..." },
  "cpu": { "model": "string", "cores": 64 },
  "ram_bytes": 549755813888,
  "disk_bytes_free": 1099511627776,
  "gpus": [
    { "vendor": "nvidia", "name": "string", "uuid": "string", "memory_bytes": 85899345920, "driver": "string" }
  ],
  "agent_version": "0.1.0",
  "reported_at": "2026-10-13T04:00:00Z"
}
```

Utilisation sample batch:

```json
{
  "machine_id": "string",
  "samples": [
    {
      "at": "2026-10-13T04:01:00Z",
      "cpu_pct": 12.5,
      "ram_used_bytes": 103079215104,
      "gpus": [ { "uuid": "string", "util_pct": 3.0, "memory_used_bytes": 1073741824 } ]
    }
  ]
}
```

### Database tables

`machines`, `gpus`, `machine_sources`, `utilisation_samples` (partitioned or indexed by time, 14-day retention job), `connectors`, `scans`, `agents`, `enrolment_tokens`, `admin_users`. Use SQL migrations in `/internal/store/migrations`.

### Web app

- **Setup:** add a Kubernetes or AWS connector (with the read-only policy shown to copy), create an enrolment token, and copy the one-line agent install command.
- **Report:** the headline page.
  - Totals: machines, GPUs grouped by model, total GPU memory, total RAM.
  - Idle: "Your GPUs sat idle X% of the past 7 days", and when (e.g. "most nights after 7pm and all weekend").
  - A note that model recommendations arrive in the next release.
- **Machines:** table of machines, sortable by GPU memory and idle %, with source badges.
- **Machine detail:** hardware, sources, and a simple 7-day idle pattern grid (day × hour).
- Plain English everywhere. Unknown values say "unknown", never blank.

## Testing

- Unit tests for every package. Table-driven tests for merge, idle calculation and connector parsing.
- Connectors are tested against mocked SDK clients and recorded fixture responses; no real cloud account needed for CI.
- Kubernetes connector integration test with a `kind` cluster: label and patch node capacity to simulate GPU nodes (`nvidia.com/gpu`, `nvidia.com/gpu.product`, `nvidia.com/gpu.memory`).
- End-to-end test: `docker compose up`, run three fake agents from fixtures, assert the report API returns the expected totals and idle %.
- Manual check on one real NVIDIA GPU machine (owned or a rented cloud GPU instance) before calling Phase 1 done.

## Acceptance criteria

1. `docker compose up` starts Postgres, server and web app with no manual steps beyond setting the admin password.
2. A fake agent appears in the Machines list within 60 seconds of starting.
3. On a real NVIDIA GPU machine, the agent reports the correct GPU name and memory, and utilisation changes when a GPU load is run.
4. Adding a Kubernetes connector to a `kind` cluster with simulated GPU nodes lists every node with the right GPU count and model.
5. Adding an AWS connector (mocked in CI) lists instances with GPU name, count and memory from `DescribeInstanceTypes`.
6. A machine seen by both AWS and an agent appears once, with both sources shown.
7. After 24+ hours of fake samples, the report shows idle % and an idle pattern matching the fixture's schedule.
8. Pulling the network on an agent does not crash it; samples buffer locally (up to 24 hours) and send when back online.
9. No secret or credential appears in logs.
10. All tests pass with `make test`; `make lint` is clean.

## Definition of done

All acceptance criteria pass, the README explains install in under 10 steps, and the scan report reads clearly to someone with no ML background.
