# Build stage
FROM golang:1.25-alpine AS builder

# Install git and ca-certificates (needed for go mod download)
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags="-s -w -X main.Version=docker -X main.CommitHash=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')" \
    -o astrolavos .

# Runtime stage - use distroless for security
FROM gcr.io/distroless/static:nonroot

# Copy CA certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /src/astrolavos /astrolavos

# Use non-root user
USER 65532:65532

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/astrolavos", "-oneoff", "-config-path", "/dev/null"] || exit 1

ENTRYPOINT ["/astrolavos"]
