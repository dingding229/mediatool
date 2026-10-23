SHELL := /bin/bash
.PHONY: init dev build build-web test test-integration vet run worker migrate compose-check
init:
	go run ./cmd/media-tool-dev init-env
dev:
	bash scripts/dev.sh
build-web:
	bash scripts/build-web.sh
build: build-web
	mkdir -p bin
	eval "$$(go run ./cmd/media-tool-dev build-meta env)"; CGO_ENABLED=0 go build -trimpath -ldflags="$$MEDIA_TOOL_LDFLAGS" -o bin/media-tool ./cmd/media-tool
	CGO_ENABLED=0 go build -trimpath -ldflags="$$(go run ./cmd/media-tool-dev build-meta ldflags)" -o bin/media-toolctl ./cmd/media-toolctl
	CGO_ENABLED=0 go build -trimpath -o bin/media-tool-dev ./cmd/media-tool-dev
test: build-web
	bash scripts/test-go.sh
	npm test --prefix web
test-integration: build-web
	bash scripts/test-go.sh
vet:
	go vet ./...
run:
	bash scripts/load-env.sh go run ./cmd/media-tool
worker:
	bash scripts/load-env.sh go run ./cmd/media-tool -role worker
migrate:
	bash scripts/load-env.sh go run ./cmd/media-tool migrate
compose-check:
	docker compose -f docker-compose.yml config --quiet
