# =============================================================================
# Stage 1 — Build the Go binary
# =============================================================================
# Use the official Go image to compile a statically-linked binary.
# We separate dependency download from source copy to leverage Docker layer
# caching: go.mod/go.sum rarely change, so this layer is almost always a hit.
FROM golang:1.27-alpine AS builder

WORKDIR /app

# 1. Download dependencies first (cache-friendly layer)
COPY go.mod go.sum ./
RUN go mod download

# 2. Copy the full source tree and compile
COPY . .
RUN CGO_ENABLED=0 go build -o /app/bin/navier-stocks ./src/main

# =============================================================================
# Stage 2 — Minimal runtime image
# =============================================================================
# Copy only the compiled binary into a tiny Alpine base.  No Go toolchain,
# no compiler, no package manager — just ca-certificates for TLS and the binary.
FROM alpine:3.24

# ca-certificates are needed if the app makes outbound HTTPS calls (e.g. broker API)
RUN apk add --no-cache ca-certificates

# Copy the binary from the builder stage
COPY --from=builder /app/bin/navier-stocks /usr/local/bin/

# Default command — runs all four agents in parallel within a single process.
# Environment variables (NATS_URL, PG_*, TRADER_MODE, etc.) are injected at
# runtime by docker compose or the container orchestrator.
CMD ["navier-stocks"]
