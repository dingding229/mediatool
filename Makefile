SHELL := /bin/bash
.PHONY: init dev build build-web test test-integration vet run worker migrate compose-check
init:
	python3 scripts/init-env.py
dev:
	bash scripts/dev.sh
build-web:
	bash scripts/build-web.sh
build: build-web
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/media-tool ./cmd/media-tool
	CGO_ENABLED=0 go build -trimpath -o bin/migrate ./cmd/migrate
test:
	go test -p 1 -race ./...
	npm ci --prefix web --no-fund
	npm test --prefix web
test-integration:
	bash scripts/test-go.sh
vet:
	go vet ./...
run:
	bash scripts/load-env.sh go run ./cmd/media-tool
worker:
	bash scripts/load-env.sh go run ./cmd/media-tool -role worker
migrate:
	bash scripts/load-env.sh go run ./cmd/migrate
compose-check:
	docker compose -f docker-compose.yml config --quiet
