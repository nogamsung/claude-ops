# --- Build stage ---
FROM golang:1.25-alpine AS builder

# Build-time metadata (injected via --build-arg from CI)
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /build

# Dependency layer — cached unless go.mod/go.sum change
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO is no longer required — pure-Go go-sql-driver/mysql.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
      -X main.Version=${VERSION} \
      -X main.Commit=${COMMIT} \
      -X main.BuildDate=${BUILD_DATE}" \
    -o /bin/claude-ops \
    ./cmd/claude-ops/

# --- Runtime stage ---
FROM alpine:3.19

# Install runtime dependencies.
# Note: claude CLI and gh CLI must be installed separately on the host
# and shared via volume mounts (see docker-compose.yml). MySQL runs as a
# sibling service in compose, not inside this image.
RUN apk add --no-cache \
    git \
    ca-certificates \
 && addgroup -S agent \
 && adduser -S agent -G agent

COPY --from=builder /bin/claude-ops /usr/local/bin/claude-ops

# Default directories — owned by non-root user
RUN mkdir -p /worktrees /prompts /migrations \
 && chown -R agent:agent /worktrees /prompts /migrations

WORKDIR /app

COPY migrations/ /migrations/
COPY prompts/ /prompts/

# Switch to non-root user
USER agent

EXPOSE 8787

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8787/healthz || exit 1

ENTRYPOINT ["claude-ops"]
CMD ["-config", "/config/config.yaml"]
