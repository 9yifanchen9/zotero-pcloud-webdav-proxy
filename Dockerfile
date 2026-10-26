# syntax=docker/dockerfile:1

# Cross-compiles on the build host's architecture, so multi-platform builds need no emulation
FROM --platform=$BUILDPLATFORM golang:1.27@sha256:f20d83a83e18dbd2a4714b8ffd4b30b287b6243962dd75608e86abcc63ba4efc AS build
ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/zotero-pcloud-webdav-proxy .

# Static base with CA certificates for TLS to pCloud and a non-root user, and no shell
FROM gcr.io/distroless/static-debian13:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99
COPY --from=build /out/zotero-pcloud-webdav-proxy /zotero-pcloud-webdav-proxy
# A published port only reaches the proxy if it listens on all interfaces
ENV LISTEN_ADDR=0.0.0.0:8080
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/zotero-pcloud-webdav-proxy", "healthcheck"]
ENTRYPOINT ["/zotero-pcloud-webdav-proxy"]
