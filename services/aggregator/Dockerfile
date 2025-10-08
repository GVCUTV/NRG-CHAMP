# v4
# file: Dockerfile
FROM golang:1.23-alpine AS agg-builder
WORKDIR /app
COPY services/aggregator/go.mod services/aggregator/go.sum ./
COPY circuit_breaker /circuit_breaker
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY services/aggregator/ ./
RUN CGO_ENABLED=0 go build -o /aggregator ./cmd/server

FROM alpine:3.20 AS agg-runtime
WORKDIR /app
RUN adduser -D -H appuser
COPY --from=agg-builder /aggregator /app/aggregator
COPY --from=agg-builder /app/aggregator.properties /app/aggregator.properties
ENV AGG_PROPERTIES=/app/aggregator.properties
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=2s --retries=5 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/aggregator"]
