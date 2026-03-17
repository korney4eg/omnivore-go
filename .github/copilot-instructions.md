# Copilot Instructions

## Project goal

This repository is a Go rewrite of the Omnivore backend. The original TypeScript monorepo lives at `/Users/aliaksei.karneyeu/projects/omnivore` (its CLAUDE.md documents commands and architecture). The long-term aim is to rewrite two things:

1. **The API** — currently `packages/api` (Apollo/Express GraphQL on port 4000, TypeORM + PostgreSQL, BullMQ workers)
2. **The background workers** — currently `packages/api/src/queue-processor.ts`, which processes jobs from `omnivore-backend-queue`

The existing TypeScript queue processor handles ~20 job types: `save-page`, `ai-summarize`, `bulk-action`, `call-webhook`, `send-email`, `find-thumbnail`, `trigger-rule`, `export`, `prune-trash`, `upload-content`, `update-home`, `create-digest`, and others in `packages/api/src/jobs/`. The Go rewrite should implement these on the same `omnivore-backend-queue` BullMQ queue so the TS and Go workers remain interoperable during the transition.

When implementing new Go behaviour, consult the original TypeScript source in `/Users/aliaksei.karneyeu/projects/omnivore` to preserve exact wire formats, queue semantics, and business logic. The same module name (`github.com/omnivore-app/omnivore`) exists in both repos — `src-go/` inside the monorepo mirrors this repo and may contain in-progress work.

## Build and test commands

Convenience targets are defined in the `Makefile`:

| Target | What it does |
|---|---|
| `make content_fetch_go` | Run the service via `go run . server content-fetcher` |
| `make content_fetch_go_build` | Compile to `bin/omnivore` |
| `make docker_build_content_fetcher` | Build the Docker image (see below) |
| `make docker_push_content_fetcher` | Build and push (`REGISTRY` and `IMAGE_TAG` are overridable) |

Raw Go toolchain equivalents:

- Build: `go build ./...`
- Test all: `go test ./...`
- Single test: `go test ./internal/handler -run TestJobData_UnmarshalLabelsAsObjects -count=1`

**Docker:** `docker/content-fetcher.Dockerfile` uses the repository root as the build context. Build with `make docker_build_content_fetcher` (or `docker build -f docker/content-fetcher.Dockerfile -t <image> .`). The runtime image uses Alpine with Chromium from the edge repo and appends a host-based ad/tracker blocklist at startup.

All configuration is provided via environment variables. Copy `.env` from the repository root and fill in the required values — it contains every supported variable with defaults. The minimum required at startup are `VERIFICATION_TOKEN` and `REDIS_URL`. The service defaults to port 3002 (override with `PORT`).

## Running the deploy stack

"Bring up the deploy version" means: run `docker compose up -d` from the `deploy/` folder. This starts the full Omnivore stack using the reference images (TS backend, queue processor, web, image proxy, mail watcher, Redis, Postgres, MinIO).

```bash
cd deploy
docker compose up -d
```

To tear it down cleanly (including volumes):

```bash
cd deploy
docker compose down -v
```

The `deploy/` folder contains:
- `docker-compose.yml` — reference stack using pre-built images; `content-fetch` uses the published `korney4eg/omnivore-content-fetcher` image (not the local build)
- `.env` — environment config for the whole stack
- `setup_db.bash` must exist at the **repository root** — it is mounted into the migrate container at startup to initialise the database schema

## High-level architecture

This repository is a single Go binary (`main.go`) that dispatches through Cobra commands in `cmd/`. Today the only implemented service command is `omnivore server content-fetcher` (`cmd/server/content_fetcher.go`).

The `content-fetcher` command wires together the whole service lifecycle:

- load env-driven config from `internal/config`
- connect Redis clients with `internal/redisutil`
- create a shared headless browser allocator from `internal/browser`
- start the BullMQ-compatible worker in `internal/queue` (concurrency 4, polls every 500 ms)
- expose HTTP endpoints from `internal/server`

There are two entry paths into the same core processing logic:

- HTTP requests to `GET`/`POST /?token=...` in `internal/server/server.go`
- Redis queue jobs popped from `omnivore-content-fetch-queue` by `internal/queue/worker.go`

Both paths call `handler.ProcessFetchContentJob` in `internal/handler/handler.go`. That function is the orchestration center for the service:

- normalize request/job payloads into shared `JobData`
- block bad domains and consult Redis-backed fetch failure counters
- reuse cached fetch results from Redis when available
- fetch pages through `internal/fetch`, which uses `chromedp` and special-cases PDFs and anti-bot detection
- upload original HTML to blob storage through `internal/storage`
- enqueue `save-page` jobs onto `omnivore-backend-queue` through `internal/bullmq`
- emit failure analytics through `internal/analytics`
- optionally notify the importer metrics collector

`internal/bullmq` is intentionally not a generic queue abstraction. It reproduces the Redis key layout and job state transitions expected by the existing TypeScript BullMQ ecosystem, so this Go service can interoperate with existing producers and consumers.

`internal/metrics` exposes Prometheus metrics for queue depth and oldest-job age by querying Redis on each `/metrics` request.

## Key conventions

Many packages explicitly mirror behavior from the original TypeScript services. Preserve those compatibility contracts when changing payloads, queue names, retry logic, analytics event names, or storage key formats. Important examples are called out directly in comments in:

- `internal/handler/handler.go`
- `internal/fetch/fetch.go`
- `internal/bullmq/bullmq.go`
- `internal/analytics/analytics.go`

Do not treat the HTTP handler path and queue worker path as separate implementations. They intentionally converge on `handler.ProcessFetchContentJob`; shared behavior should stay there unless there is a strong reason to split it.

`internal/config.Config` is env-only configuration. Prefer adding new settings there instead of reading environment variables ad hoc in other packages.

For object storage, prefer `BLOB_STORAGE_URL`. `Config.BlobURL()` keeps legacy `GCS_UPLOAD_BUCKET` behavior for backward compatibility, and `handler.ProcessFetchContentJob` still bridges `GCS_UPLOAD_SA_KEY_FILE_PATH` into `GOOGLE_APPLICATION_CREDENTIALS`.

Redis is split by responsibility: `RedisDataSource.CacheClient` is for fetch caching and failure counters, while `MQClient` is for BullMQ state. `MQClient` may intentionally alias `CacheClient` when only one Redis URL is configured.

Tests are lightweight package tests, not integration suites. `internal/storage/storage_test.go` uses `memblob`, so storage changes should usually keep tests dependency-free rather than requiring Docker or cloud credentials.

The handler tests verify JSON compatibility, especially that labels are objects rather than strings. Preserve those wire formats when editing `JobData` or `savePageJobData`.
