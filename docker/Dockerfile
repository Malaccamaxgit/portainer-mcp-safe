# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.24.13
ARG DEBIAN_VERSION=bookworm-slim
ARG PORTAINER_MCP_SAFE_VERSION=0.7.0-safe.1

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS base

WORKDIR /src

COPY go.mod go.sum ./
RUN /usr/local/go/bin/go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
COPY docker/portainer-mcp-gateway.yaml ./docker/portainer-mcp-gateway.yaml

FROM base AS test

ENV CGO_ENABLED=0
CMD ["/usr/local/go/bin/go", "test", "-count=1", "./cmd/...", "./internal/...", "./pkg/..."]

FROM base AS builder

ARG TARGETOS
ARG TARGETARCH
ARG PORTAINER_MCP_SAFE_VERSION

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} /usr/local/go/bin/go build -buildvcs=false -ldflags "-s -w -X main.Version=${PORTAINER_MCP_SAFE_VERSION}" -o /out/portainer-mcp ./cmd/portainer-mcp

FROM debian:${DEBIAN_VERSION} AS runtime

ARG PORTAINER_MCP_SAFE_VERSION

RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --home-dir /home/mcp --shell /usr/sbin/nologin mcp \
    && mkdir -p /tmp \
    && chown -R mcp:mcp /home/mcp /tmp

COPY --from=builder /out/portainer-mcp /usr/local/bin/portainer-mcp
COPY docker/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod 0555 /usr/local/bin/portainer-mcp /usr/local/bin/docker-entrypoint.sh

USER mcp
WORKDIR /home/mcp

ENV PORTAINER_TOOLS_PATH=/tmp/portainer-tools.yaml

LABEL org.opencontainers.image.title="Portainer MCP Safe Docker Toolkit wrapper"
LABEL org.opencontainers.image.description="Container wrapper for an altered-source Portainer MCP fork, suitable for Docker MCP Toolkit and MCP Gateway."
LABEL org.opencontainers.image.source="https://github.com/Malaccamaxgit/portainer-mcp-safe"
LABEL org.opencontainers.image.licenses="zlib"
LABEL org.opencontainers.image.version="${PORTAINER_MCP_SAFE_VERSION}"

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
