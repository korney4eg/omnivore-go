# ── Configuration (override on the command line: make <target> REGISTRY=myrepo) ─
REGISTRY     ?= korney4eg
IMAGE_TAG    ?= latest
OMNIVORE_URL ?= http://omnivore-deploy

# ── Go content-fetcher ──────────────────────────────────────────────────────

content_fetch_go:
	go run . server content-fetcher

content_fetch_go_build:
	go build -o bin/omnivore .

# ── Docker images ───────────────────────────────────────────────────────────

docker_build_content_fetcher:
	docker build -f docker/content-fetcher.Dockerfile \
		-t $(REGISTRY)/omnivore-content-fetcher:$(IMAGE_TAG) .

docker_push_content_fetcher: docker_build_content_fetcher
	docker push $(REGISTRY)/omnivore-content-fetcher:$(IMAGE_TAG)

# ── Tests ───────────────────────────────────────────────────────────────────

# Run integration tests against OMNIVORE_URL (default: http://omnivore-deploy).
# Override: make test_integration OMNIVORE_URL=http://omnivore-dev
test_integration:
	OMNIVORE_URL=$(OMNIVORE_URL) go test ./tests/integration/... -v -timeout 5m

# Reset the deploy stack (wipes DB + volumes) then run integration tests.
test_integration_clean:
	docker compose -f deploy/docker-compose.yml --env-file deploy/.env down -v --remove-orphans
	docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d
	@echo "Waiting for stack to be ready..."
	@sleep 30
	OMNIVORE_URL=$(OMNIVORE_URL) go test ./tests/integration/... -v -timeout 5m
