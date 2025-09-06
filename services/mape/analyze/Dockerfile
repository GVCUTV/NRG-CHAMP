# Stage 1: Build
FROM golang:1.21-alpine AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o analyze-app ./cmd

# Stage 2: Minimal runtime
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/analyze-app .

ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8083/health || exit 1

EXPOSE 8083

ENTRYPOINT ["./analyze-app"]
