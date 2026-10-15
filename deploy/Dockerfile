FROM --platform=$BUILDPLATFORM node:22-alpine@sha256:0a7108bf6c7bf5de370ffb1a3ed6be93d405b43ff159f681a8d18c0e2bc2e402 AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.9-alpine@sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0 AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/internal/platform/webui/assets/ ./internal/platform/webui/assets/
ARG TARGETOS=linux
ARG TARGETARCH
ARG SOURCE_REVISION
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X media-tool/internal/platform/buildinfo.Revision=${SOURCE_REVISION}" -o /out/media-tool ./cmd/media-tool && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X media-tool/internal/platform/buildinfo.Revision=${SOURCE_REVISION}" -o /out/ops-control ./cmd/ops-control && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/backup-verify ./cmd/backup-verify && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/rekey ./cmd/rekey && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/admin-reset ./cmd/admin-reset && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/cloud-upload-smoke ./cmd/cloud-upload-smoke && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/site-compat-audit ./cmd/site-compat-audit && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/site-template-sync ./cmd/site-template-sync && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/cloud-events-check ./cmd/cloud-events-check

FROM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0 AS runtime
RUN apk add --no-cache ca-certificates postgresql18-client && addgroup -S -g 65532 app && adduser -S -u 65532 -G app app && mkdir -p /data/strm /data/downloads /data/files && chown -R app:app /data
LABEL org.opencontainers.image.schema="50"
WORKDIR /app
COPY --from=backend /out/media-tool /app/media-tool
COPY --from=backend /out/ops-control /app/ops-control
COPY --from=backend /out/migrate /app/migrate
COPY --from=backend /out/backup-verify /app/backup-verify
COPY --from=backend /out/rekey /app/rekey
COPY --from=backend /out/admin-reset /app/admin-reset
COPY --from=backend /out/cloud-upload-smoke /app/cloud-upload-smoke
COPY --from=backend /out/site-compat-audit /app/site-compat-audit
COPY --from=backend /out/site-template-sync /app/site-template-sync
COPY --from=backend /out/cloud-events-check /app/cloud-events-check
USER 65532:65532
EXPOSE 8080 8091
ENTRYPOINT ["/app/media-tool"]
