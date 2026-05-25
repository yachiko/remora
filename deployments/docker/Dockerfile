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
FROM gcr.io/distroless/static-debian13:nonroot

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/remora /app/remora

# Copy timezone data from builder
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Expose port
EXPOSE 8080

# Run the application
ENTRYPOINT ["/app/remora"]
