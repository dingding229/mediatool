FROM alpine:3.23.8@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2 AS ip2region-data
RUN apk add --no-cache ca-certificates curl
COPY scripts/prepare-ip2region.sh /prepare-ip2region.sh
RUN sh /prepare-ip2region.sh /out

FROM --platform=$BUILDPLATFORM golang:1.26.11-alpine@sha256:ecb67f987aa1a57794c91d19ade7ae0209f2f0f5496aaef9e6ae85ddf92d02ab AS plugin-resources
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go run ./cmd/media-tool-dev plugins check && go run ./cmd/media-tool-dev plugins assets --check && go run ./cmd/media-tool-dev plugins assets

FROM --platform=$BUILDPLATFORM node:22-alpine@sha256:1314fb30ec37592d5845e5a160d522c0e680972aa534549940b8d73bde88481a AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
COPY --from=plugin-resources /src/web/public/ ./public/
COPY --from=plugin-resources /src/web/plugin-ui/ ./plugin-ui/
# D1 Vue tests/typechecks import the same public contract fixtures as Go.
COPY internal/platform/httpapi/testdata/contracts/ /src/internal/platform/httpapi/testdata/contracts/
COPY internal/modules/plugins/v2/library.json /src/internal/modules/plugins/v2/library.json
COPY internal/modules/messaging/testdata/notice-display.json /src/internal/modules/messaging/testdata/notice-display.json
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.11-alpine@sha256:ecb67f987aa1a57794c91d19ade7ae0209f2f0f5496aaef9e6ae85ddf92d02ab AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/internal/platform/webui/assets/ ./internal/platform/webui/assets/
ARG TARGETOS=linux
ARG TARGETARCH
ARG SOURCE_REVISION
ARG SOURCE_VERSION
ARG SOURCE_BUILD_TIME
RUN test -n "$SOURCE_VERSION" && test -n "$SOURCE_REVISION" && test -n "$SOURCE_BUILD_TIME" && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X media-tool/internal/platform/buildinfo.Version=${SOURCE_VERSION} -X media-tool/internal/platform/buildinfo.Commit=${SOURCE_REVISION} -X media-tool/internal/platform/buildinfo.BuildTime=${SOURCE_BUILD_TIME}" -o /out/media-tool ./cmd/media-tool && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X media-tool/internal/platform/buildinfo.Version=${SOURCE_VERSION} -X media-tool/internal/platform/buildinfo.Commit=${SOURCE_REVISION} -X media-tool/internal/platform/buildinfo.BuildTime=${SOURCE_BUILD_TIME}" -o /out/media-toolctl ./cmd/media-toolctl

FROM alpine:3.23.8@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2 AS runtime
RUN apk add --no-cache ca-certificates postgresql18-client ffmpeg docker-cli docker-cli-compose && addgroup -S -g 65532 app && adduser -S -u 65532 -G app app && mkdir -p /data/plugins && mkdir -p /data/strm /data/downloads /data/files /data/native-media/stills && chown -R app:app /data
LABEL org.opencontainers.image.schema="97"
WORKDIR /app
COPY --from=backend /out/media-tool /app/media-tool
COPY --from=backend /out/media-toolctl /app/media-toolctl
COPY --from=ip2region-data /out/ /app/ip2region/
USER 65532:65532
EXPOSE 8080 8091 8092
ENTRYPOINT ["/app/media-tool"]
