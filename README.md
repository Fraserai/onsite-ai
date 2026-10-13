# Onsite AI — Scan and Report

Finds every machine and GPU you own, learns when each one sits idle, and shows
it in one plain-English report. Everything runs inside your own environment:
the scan is read-only and no data leaves it.

This repository is Phase 1 of the product. See [SPEC.md](SPEC.md) for the full
spec and [CLAUDE.md](CLAUDE.md) for the working rules.

> **Status:** under construction. The server, database and web shell run;
> agents, connectors and the report pages are landing milestone by milestone.

## Run it locally

You need Docker (Docker Desktop, OrbStack or Colima) and `make`.

1. Start your container runtime (for Colima: `colima start`).
2. From this folder, run `make dev`.
3. Open the web app at <http://localhost:8080>.

`make dev` starts Postgres, the server and the web app, and saves the server's
certificate authority to `.dev/ca.pem`. Stop everything with `make down`; your
data is kept.

## How the pieces talk

- **Server** — `https://localhost:8443`. HTTPS only. With no certificate
  configured it creates its own certificate authority on first start and issues
  itself a certificate. Agents trust the server through that CA file.
- **Web app** — `http://localhost:8080`, reachable from this machine only. It
  passes API calls to the server over verified HTTPS.
- **Postgres** — internal to the Compose network; not published.

## Server settings

| Variable | Default | Purpose |
| --- | --- | --- |
| `ONSITE_DATABASE_URL` | (required) | Postgres connection string |
| `ONSITE_LISTEN_ADDR` | `:8443` | Address the HTTPS server listens on |
| `ONSITE_DATA_DIR` | `data` | Where the server keeps its private files |
| `ONSITE_PUBLIC_DIR` | `<data dir>/public` | Where the public CA certificate is written |
| `ONSITE_TLS_HOSTS` | (none) | Extra names or IPs for the self-signed certificate, comma-separated |
| `ONSITE_TLS_CERT_FILE`, `ONSITE_TLS_KEY_FILE` | (none) | Use your own certificate instead of the self-signed one |

For agents on other machines, set `ONSITE_TLS_HOSTS` to the name they will use
to reach this server before running `make dev`.

## Developing

You need Go, Node.js and `golangci-lint` as well as Docker.

| Command | What it does |
| --- | --- |
| `make dev` | Run everything locally via Docker Compose |
| `make test` | All Go and web tests (starts a throwaway test database) |
| `make lint` | `golangci-lint` and the web linter |
| `make agent-fake` | Run an agent in fake mode from a fixture |
| `make help` | List every command |

## Layout

```
cmd/agent            agent binary
cmd/server           server binary
internal/agent       hardware collection
internal/connectors  kubernetes/, aws/
internal/inventory   machine merge, de-duplication, idle calculation
internal/api         HTTP handlers and request validation
internal/store       Postgres access and migrations
web                  React + TypeScript app
testdata/fixtures    fake machines and samples
deploy               Docker Compose files, Dockerfiles
```
