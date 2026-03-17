# ── Configuration (override on the command line: make <target> REGISTRY=myrepo) ─
# ── Configuration (override on the command line: make <target> REGISTRY=myrepo) ─
REGISTRY     ?= korney4eg
IMAGE_TAG    ?= latest
OMNIVORE_URL ?= http://omnivore-deploy
DEPLOY_DIR   ?= deploy
HUGO_DIR     ?= /Users/aliaksei.karneyeu/projects/makvaz.com
TESTSITE_DIR ?= testsite

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

# ── Deploy stack ────────────────────────────────────────────────────────────

# Bring up the reference deploy stack (uses published images, NOT the local build).
deploy_up:
	docker compose -f $(DEPLOY_DIR)/docker-compose.yml --env-file $(DEPLOY_DIR)/.env up -d

# Tear down the deploy stack and wipe all volumes (clean state).
deploy_down:
	docker compose -f $(DEPLOY_DIR)/docker-compose.yml --env-file $(DEPLOY_DIR)/.env down -v --remove-orphans

# ── Dev stack ───────────────────────────────────────────────────────────────

DEV_DIR ?= dev

# Bring up the dev stack (builds content-fetcher from local Dockerfile).
dev_up:
	docker compose -f $(DEV_DIR)/docker-compose.yml --env-file $(DEV_DIR)/.env up -d --build

# Tear down the dev stack and wipe all volumes (clean state).
dev_down:
	docker compose -f $(DEV_DIR)/docker-compose.yml --env-file $(DEV_DIR)/.env down -v --remove-orphans

# ── Testsite ────────────────────────────────────────────────────────────────

# Build the EN-only Hugo static site into testsite/public/ and start the nginx
# container on 0.0.0.0:8765. The test process reaches it via http://localhost:8765;
# Docker containers reach it via extra_hosts entry: omnivore-testsite → host-gateway.
# Note: integration tests (test_integration / test_integration_clean) run this
# automatically via TestMain — you only need this target for manual inspection.
testsite_start:
	hugo --source $(HUGO_DIR) --baseURL http://omnivore-testsite:8765 \
		--destination $(PWD)/$(TESTSITE_DIR)/public --environment production
	docker compose -f $(TESTSITE_DIR)/docker-compose.yml up -d --wait

testsite_stop:
	docker compose -f $(TESTSITE_DIR)/docker-compose.yml down

# ── Tests ───────────────────────────────────────────────────────────────────

# Run integration tests against OMNIVORE_URL (default: http://omnivore-deploy).
# TestMain automatically builds Hugo and starts/stops the testsite container.
# The deploy stack must already be running. Override host:
#   make test_integration OMNIVORE_URL=http://omnivore-dev
test_integration:
	OMNIVORE_URL=$(OMNIVORE_URL) HUGO_DIR=$(HUGO_DIR) go test ./tests/integration/... -v -timeout 5m

# Reset the deploy stack (wipes DB + volumes) then run integration tests.
# Use this for a guaranteed clean run; test_integration works on dirty DB too.
# To test against dev stack: make test_integration_clean DEPLOY_DIR=dev OMNIVORE_URL=http://omnivore-dev
test_integration_clean: deploy_down deploy_up
	@echo "Waiting 30s for stack to initialise..."
	@sleep 30
	OMNIVORE_URL=$(OMNIVORE_URL) HUGO_DIR=$(HUGO_DIR) go test ./tests/integration/... -v -timeout 5m

