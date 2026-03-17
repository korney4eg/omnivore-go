# ── Configuration (override on the command line: make <target> REGISTRY=myrepo) ─
REGISTRY ?= korney4eg
IMAGE_TAG ?= latest

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
