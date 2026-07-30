# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false \
    -ldflags="-X 'github.com/joaojsr/shiori-server/internal/buildinfo.Version=docker' \
              -X 'github.com/joaojsr/shiori-server/internal/buildinfo.Commit=$(git rev-parse HEAD 2>/dev/null || echo unknown)' \
              -X 'github.com/joaojsr/shiori-server/internal/buildinfo.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)'" \
    -o shiori-server ./cmd/api

# Final stage
FROM alpine:3.19

# Install CA certificates for external HTTPS calls
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/shiori-server /app/shiori-server

# Non-root user
RUN addgroup -S shiori && adduser -S shiori -G shiori
USER shiori

EXPOSE 9180

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:9180/health/ready || exit 1

ENTRYPOINT ["/app/shiori-server"]
CMD ["serve", "--profile", "docker", "--log-format", "json"]
