# --- Build stage ---
FROM golang:1.22-alpine AS builder

# Build-time metadata (injected via --build-arg from CI)
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# CGO_ENABLED=1 is required: mattn/go-sqlite3 uses cgo
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build

# Dependency layer — cached unless go.mod/go.sum change
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w \
      -X main.Version=${VERSION} \
      -X main.Commit=${COMMIT} \
      -X main.BuildDate=${BUILD_DATE}" \
    -o /bin/scheduled-dev-agent \
    ./cmd/scheduled-dev-agent/

# --- Runtime stage ---
FROM alpine:3.19

# Install runtime dependencies.
# Note: claude CLI and gh CLI must be installed separately on the host
# and shared via volume mounts (see docker-compose.yml).
RUN apk add --no-cache \
    git \
    ca-certificates \
    sqlite \
 && addgroup -S agent \
 && adduser -S agent -G agent

COPY --from=builder /bin/scheduled-dev-agent /usr/local/bin/scheduled-dev-agent

# Default directories — owned by non-root user
RUN mkdir -p /data /worktrees /prompts /migrations \
 && chown -R agent:agent /data /worktrees /prompts /migrations

WORKDIR /app

COPY migrations/ /migrations/
COPY prompts/ /prompts/

# Switch to non-root user
USER agent

EXPOSE 8787

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8787/health || exit 1

ENTRYPOINT ["scheduled-dev-agent"]
CMD ["-config", "/config/config.yaml"]
