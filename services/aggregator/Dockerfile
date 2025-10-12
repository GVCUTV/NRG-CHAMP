# // v7
# // file: services/aggregator/Dockerfile
FROM golang:1.23-alpine AS aggregator_build
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.work go.work.sum ./
COPY services/aggregator/go.mod services/aggregator/go.sum ./services/aggregator/
RUN --mount=type=cache,target=/go/pkg/mod \
    go -C services/aggregator mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go -C services/aggregator build -o /aggregator ./cmd/server

FROM alpine:3.20 AS aggregator_runtime
WORKDIR /app
RUN adduser -D -H appuser
COPY --from=aggregator_build /aggregator /app/aggregator
COPY --from=aggregator_build /app/services/aggregator/aggregator.properties /app/aggregator.properties
ENV AGG_PROPERTIES=/app/aggregator.properties
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=2s --retries=5 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/aggregator"]
