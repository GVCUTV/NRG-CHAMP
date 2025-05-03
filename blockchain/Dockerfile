# Stage 1: Build the blockchain archiviation service
FROM golang:1.21-alpine AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

# Copy go.mod and go.sum (from root or local module)
COPY go.mod go.sum ./
RUN go mod download

# Copy the full project to resolve imports
COPY . .

# Build the blockchain binary
RUN go build -o blockchain-app ./cmd

# Stage 2: Minimal runtime container
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/blockchain-app .

# Logging via env vars
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

# Health check via HTTP (adjust port/endpoint if needed)
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8087/health || exit 1

EXPOSE 8087

ENTRYPOINT ["./blockchain-app"]
