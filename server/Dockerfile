# ==============================================================================
# DOCKERFILE MULTI-STAGE DURCI — AIGLE SERVER (EDITION SAAS MULTI-TENANT)
# Inclut la compilation croisée des binaires clients téléchargeables (Win/Mac/Linux)
# Compatible : Docker, Podman (rootless), Kubernetes (CRI-O / containerd)
# ==============================================================================

# ------------------------------------------------------------------------------
# STAGE 1 : Compilation du Frontend Svelte 5 + Tailwind v4 (avec Bun)
# ------------------------------------------------------------------------------
FROM oven/bun:1-alpine AS frontend-builder
WORKDIR /build/web

COPY web/package.json ./
RUN bun install

COPY web/ ./
RUN bun run build

# ------------------------------------------------------------------------------
# STAGE 2 : Compilation du Serveur SaaS + Packaging des Binaires Clients
# ------------------------------------------------------------------------------
FROM golang:alpine AS backend-builder
ENV GOTOOLCHAIN=auto
WORKDIR /build

RUN apk add --no-cache ca-certificates tzdata git

COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/embed.go ./web/embed.go
COPY --from=frontend-builder /build/web/dist ./web/dist

# Copie des binaires clients multi-OS et présentations pré-générés dans downloads/
COPY downloads/ /build/downloads/

# 1. Compilation du serveur SaaS (Linux amd64 avec driver PostgreSQL + SQLite)
# 2. Si les binaires clients ne sont pas déjà présents dans downloads/, les compile en fallback
RUN mkdir -p /build/downloads && \
    sed -i 's/^go .*/go 1.23/' go.mod && \
    sed -i '/^toolchain /d' go.mod && \
    go get github.com/lib/pq@v1.10.9 && \
    go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
      -tags postgres -trimpath -ldflags="-s -w" \
      -o /build/aigle-server ./cmd/aigle-server && \
    ([ -f /build/downloads/aigle-windows-amd64.exe ] || CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /build/downloads/aigle-windows-amd64.exe ./cmd/aigle-server) && \
    ([ -f /build/downloads/aigle-darwin-arm64 ] || CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o /build/downloads/aigle-darwin-arm64 ./cmd/aigle-server) && \
    ([ -f /build/downloads/aigle-darwin-amd64 ] || CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /build/downloads/aigle-darwin-amd64 ./cmd/aigle-server) && \
    ([ -f /build/downloads/aigle-linux-amd64 ] || cp /build/aigle-server /build/downloads/aigle-linux-amd64)

# ------------------------------------------------------------------------------
# STAGE 3 : Image de Production Minimale & Durcie (Non-Root UID 10001)
# ------------------------------------------------------------------------------
FROM alpine:3.21 AS runtime

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 10001 -S aigle && \
    adduser -u 10001 -S -G aigle -h /app -s /sbin/nologin aigle && \
    mkdir -p /app/data /app/downloads && \
    chown -R aigle:aigle /app

WORKDIR /app

# Copie du binaire serveur SaaS et du catalogue de téléchargements publics (Win/Mac/Linux/PPTX)
COPY --from=backend-builder --chown=aigle:aigle /build/aigle-server /app/aigle-server
COPY --from=backend-builder --chown=aigle:aigle /build/downloads/ /app/downloads/

USER 10001:10001

EXPOSE 8080

HEALTHCHECK --interval=20s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/aigle-server", "-healthcheck"]

ENTRYPOINT ["/app/aigle-server"]
