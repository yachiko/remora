# Multi-stage Dockerfile for Remora
# Stage 1: Builder
FROM golang:1.25-alpine AS builder

# Install build dependencies (gcc and musl-dev needed for CGO/SQLite)
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    gcc \
    musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build static binary with CGO enabled for SQLite support
# Use native architecture (works on both amd64 and arm64)
RUN CGO_ENABLED=1 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a \
    -o remora \
    ./cmd/remora

# Stage 2: Runtime
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 remora && \
    adduser -D -u 1000 -G remora remora

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/remora /app/remora

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Set ownership
RUN chown -R remora:remora /app

# Switch to non-root user
USER remora

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
ENTRYPOINT ["/app/remora"]
