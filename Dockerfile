# Multi-stage build for certctl server and agent binaries
# Stage 1: Build
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build server binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o bin/server \
    ./cmd/server

# Build agent binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o bin/agent \
    ./cmd/agent

# Stage 2: Runtime
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl

# Create non-root user
RUN addgroup -g 1000 certctl && \
    adduser -D -u 1000 -G certctl certctl

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/bin/server .
COPY --from=builder /app/bin/agent .

# Copy migration files if needed
COPY --chown=certctl:certctl migrations/ ./migrations/

# Change ownership
RUN chown -R certctl:certctl /app

# Switch to non-root user
USER certctl

# Expose port for server
EXPOSE 8443

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8443/health || exit 1

# Default entrypoint is the server
ENTRYPOINT ["/app/server"]

# Notes:
# - To run the server: docker run -p 8443:8443 -e DB_HOST=postgres certctl:latest
# - To run the agent: docker run -e SERVER_URL=http://server:8443 -e API_KEY=<key> certctl:latest /app/agent
