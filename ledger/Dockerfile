# Stage 1: Build the ledger archiviation service
FROM golang:1.22.7-alpine AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

# Copy go.mod and go.sum (from root or local module)
COPY go.mod go.sum ./
RUN go mod download

# Copy the full project to resolve imports
COPY . .

# Build the ledger binary
RUN go build -o ledger-app ./cmd/ledger

# Stage 2: Minimal runtime container
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/ledger-app .

# Logging via env vars
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

# Health check via HTTP (adjust port/endpoint if needed)
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

EXPOSE 8080

ENTRYPOINT ["./ledger-app"]
