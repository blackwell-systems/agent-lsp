# agent-lsp base image
# No language servers installed — use LSP_SERVERS env var at runtime
# or use per-language tags: ghcr.io/blackwell-systems/agent-lsp:<lang>
# See DOCKER.md for full usage.

# ── Builder stage ────────────────────────────────────────────────────────────
FROM golang:1.25-bookworm AS builder

WORKDIR /src

# Copy dependency manifests first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy full source and build a stripped static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/agent-lsp ./cmd/agent-lsp/

# ── Final stage ──────────────────────────────────────────────────────────────
FROM debian:bookworm-slim

# Install minimal runtime dependencies needed by LSP servers and their
# install scripts (ca-certificates for TLS, curl for downloads, git for VCS ops)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
    && rm -rf /var/lib/apt/lists/*

# Copy the agent-lsp binary from the builder stage
COPY --from=builder /out/agent-lsp /usr/local/bin/agent-lsp

# Copy and enable the entrypoint script (written by Agent C)
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY docker/lsp-servers.yaml /etc/agent-lsp/lsp-servers.yaml
RUN chmod +x /usr/local/bin/entrypoint.sh

# Create the LSP server cache directory used by the entrypoint for
# already-installed server binaries
RUN mkdir -p /var/cache/lsp-servers

# Create a non-root user and group for running agent-lsp.
# UID/GID 65532 is conventionally used for "nonroot" in distroless-style images.
RUN groupadd --gid 65532 nonroot \
    && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin nonroot \
    && chown -R nonroot:nonroot /var/cache/lsp-servers

# Expose the default HTTP port. Override with --port flag at runtime.
EXPOSE 8080

# Some language servers require HOME to be set.
# /tmp is writable by all users including nonroot.
ENV HOME=/tmp

RUN mkdir -p /workspace && chown nonroot:nonroot /workspace
WORKDIR /workspace

USER nonroot

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD []
