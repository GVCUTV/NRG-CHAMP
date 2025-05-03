# Stage 1: Build the Go binary
FROM golang:1.21-alpine AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the aggregator binary
RUN go build -o aggregator-app ./cmd

# Stage 2: Run with minimal image
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/aggregator-app .

# Logging environment variables (optional)
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

# Health check endpoint assumed at /health
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8081/health || exit 1

EXPOSE 8081

ENTRYPOINT ["./aggregator-app"]
