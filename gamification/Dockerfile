# Stage 1: Build the gamification binary
FROM golang:1.21-alpine AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o gamification-app ./cmd

# Stage 2: Create runtime container
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/gamification-app .

ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

# Health check for K8s readiness
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8088/health || exit 1

EXPOSE 8088

ENTRYPOINT ["./gamification-app"]
