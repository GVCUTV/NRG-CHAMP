# v3
# file: Dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o aggregator ./cmd/server

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -H appuser
COPY --from=builder /app/aggregator /app/aggregator
COPY --from=builder /app/aggregator.properties /app/aggregator.properties
ENV AGG_PROPERTIES=/app/aggregator.properties
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=2s --retries=5 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/aggregator"]
