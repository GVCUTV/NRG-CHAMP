# Stage 1: Build
FROM golang:1.21-alpine AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

# Copy module definition files
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source tree (assuming dependencies may be shared)
COPY . .

# Build the assessment binary
RUN go build -o assessment-app ./cmd

# Stage 2: Create minimal runtime image
FROM alpine:latest

WORKDIR /app

# Copy the compiled binary
COPY --from=builder /app/assessment-app .

# Logging configuration via env vars
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

# Health check for Kubernetes
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8089/health || exit 1

EXPOSE 8089

ENTRYPOINT ["./assessment-app"]
