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
| `make docker_build_content_fetcher` | Build the shared Go service image (see below) |
| `make docker_push_content_fetcher` | Build and push the shared Go service image (`REGISTRY` and `IMAGE_TAG` are overridable) |
| `make deploy_up` | Bring up the reference deploy stack |
| `make deploy_down` | Tear down the stack and wipe all volumes |
| `make dev_up` | Build the shared Go image from the root Dockerfile and bring up dev stack |
| `make dev_down` | Tear down the dev stack and wipe all volumes |
| `make test_integration` | Run integration tests (deploy stack must be running) |
| `make test_integration_clean` | Reset deploy stack + run integration tests |
| `make test_integration_clean_dev` | Reset dev stack + run integration tests against omnivore-dev:81 |
| `make testsite_start` | Build Hugo static site and start testsite nginx manually |
| `make testsite_stop` | Stop the testsite nginx container |

Configurable variables (override on command line):

| Variable | Default | Purpose |
|---|---|---|
| `OMNIVORE_URL` | `http://omnivore-deploy` | Target host for integration tests |
| `DEPLOY_DIR` | `deploy` | Path to the deploy stack folder |
| `HUGO_DIR` | `/Users/aliaksei.karneyeu/projects/makvaz.com` | Hugo source for testsite |
| `TESTSITE_DIR` | `testsite` | Testsite folder |

Raw Go toolchain equivalents:

- Build: `go build ./...`
- Test all: `go test ./...`
- Single test: `go test ./internal/handler -run TestJobData_UnmarshalLabelsAsObjects -count=1`

**Docker:** The repository root `Dockerfile` builds a single shared Go service image. Build with `make docker_build_content_fetcher` (or `docker build -f Dockerfile -t <image> .`). The runtime image uses Alpine with Chromium from the edge repo and appends a host-based ad/tracker blocklist at startup. Run different services by overriding the command, e.g. `./omnivore server api`, `./omnivore server queue-processor`, or `./omnivore server content-fetcher`.

All configuration is provided via environment variables. Copy `.env` from the repository root and fill in the required values — it contains every supported variable with defaults. The minimum required at startup are `VERIFICATION_TOKEN` and `REDIS_URL`. The service defaults to port 3002 (override with `PORT`).

## Running the deploy stack

There are two stacks:

| Stack | Folder | Hostname | Port | Go services image |
|---|---|---|---|---|
| `deploy` | `deploy/` | `omnivore-deploy` | 80 | published image `korney4eg/omnivore-content-fetcher` shared by API, queue-processor, and content-fetcher |
| `dev` | `dev/` | `omnivore-dev` | 81 | built from the root `Dockerfile` and reused by API, queue-processor, and content-fetcher |

Both stacks can run **simultaneously** without conflict:
- Dev uses port 81 (deploy uses 80)
- Dev compose runs under project name `omnivore-dev` so all container names are prefixed (`omnivore-dev-nginx-1`, etc.) — no `container_name:` overrides in `dev/`
- Each stack has its own PostgreSQL, Redis, and MinIO volumes

```bash
make deploy_up    # bring up deploy stack
make deploy_down  # tear down + wipe volumes

make dev_up       # build the shared Go image locally and bring up dev stack
make dev_down     # tear down + wipe volumes
```

Both folders have their own `.env` (gitignored). The `dev/.env` is derived from `deploy/.env` with all `omnivore-deploy` references replaced by `omnivore-dev`. `SERVER_BASE_URL` must always be `http://omnivore-api:8080` (internal Docker URL, not the public hostname).

Both stacks require `/etc/hosts` entries: `127.0.0.1 omnivore-deploy` and `127.0.0.1 omnivore-dev`.

The `setup_db.bash` at the **repository root** is mounted into the migrate container to initialise the schema and demo user (`demo@omnivore.work` / `demo_password`).

## Integration tests

Integration tests live in `tests/integration/` and exercise the full stack via GraphQL:

```bash
make test_integration                           # run against running stack
make test_integration_clean                     # reset stack then run
make test_integration OMNIVORE_URL=http://omnivore-dev  # different host
```

**TestMain** (`hugo_test.go`) runs before all tests:
1. Copies `HUGO_DIR` to a temp directory
2. Writes a single-language EN `config.toml` with `baseURL = http://omnivore-testsite:8765`
3. Injects two fresh test articles with today's date:
   - `omnivore-article-test-<HHMMSS>` — unique per run so `saveUrl` never deduplicates against a stale soft-deleted item from a prior run
   - `omnivore-rss-test` — stable daily slug so the RSS queue-processor imports it (items older than 24h are skipped on first subscription)
4. Builds Hugo static output into `testsite/public/`
5. Starts the testsite nginx container (`testsite/docker-compose.yml`) on `0.0.0.0:8765`
6. Health-checks `http://localhost:8765/feed.xml` before proceeding

The testsite is torn down after tests complete. `testsite/public/` is gitignored.

**Key design decisions:**
- Article URLs are unique per run (timestamp in slug) — Omnivore soft-deletes items and `saveUrl` would re-activate a stuck PROCESSING item if the URL was reused
- RSS test article always has today's date — the queue-processor skips items older than 24h on first subscription
- Containers reach the testsite via `extra_hosts: omnivore-testsite:host-gateway`; the host test process uses `http://localhost:8765`

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

For object storage, prefer `BLOB_STORAGE_URL`. `Config.BlobURL()` defaults self-hosted setups to a MinIO-compatible `s3://` URL and still preserves legacy `gs://` behavior when `GCS_UPLOAD_SA_KEY_FILE_PATH` is configured. `handler.ProcessFetchContentJob` still bridges `GCS_UPLOAD_SA_KEY_FILE_PATH` into `GOOGLE_APPLICATION_CREDENTIALS`.

Redis is split by responsibility: `RedisDataSource.CacheClient` is for fetch caching and failure counters, while `MQClient` is for BullMQ state. `MQClient` may intentionally alias `CacheClient` when only one Redis URL is configured.

Tests are lightweight package tests, not integration suites. `internal/storage/storage_test.go` uses `memblob`, so storage changes should usually keep tests dependency-free rather than requiring Docker or cloud credentials.

The handler tests verify JSON compatibility, especially that labels are objects rather than strings. Preserve those wire formats when editing `JobData` or `savePageJobData`.
