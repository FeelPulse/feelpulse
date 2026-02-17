# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies for CGO (sqlite)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled for SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o feelpulse ./cmd/feelpulse

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs tzdata

# Create non-root user
RUN adduser -D -h /home/feelpulse feelpulse

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/feelpulse .

# Create data directory
RUN mkdir -p /home/feelpulse/.feelpulse && \
    chown -R feelpulse:feelpulse /home/feelpulse

# Switch to non-root user
USER feelpulse

# Set home directory for config
ENV HOME=/home/feelpulse

# Expose gateway port
EXPOSE 18789

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:18789/health || exit 1

# Default command
CMD ["./feelpulse", "start"]
