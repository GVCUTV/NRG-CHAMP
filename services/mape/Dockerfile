# // v10
# // file: services/mape/Dockerfile
FROM golang:1.23-alpine AS mape_build
WORKDIR /app

COPY go.work go.work.sum ./
COPY services/mape/go.mod services/mape/go.sum ./services/mape/
COPY circuit_breaker /circuit_breaker
RUN --mount=type=cache,target=/go/pkg/mod \
    go -C services/mape mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go -C services/mape build -o /mape ./cmd/mape

FROM alpine:3.20 AS mape_runtime
WORKDIR /app
RUN adduser -D -u 10001 appuser
USER appuser
COPY --from=mape_build /mape /app/mape
COPY --from=mape_build /app/mape.properties /app/config/mape.properties
ENV HTTP_BIND=:8080
ENV LOG_DIR=/app/logs
ENV KAFKA_BROKERS=localhost:9092
ENV AGGREGATOR_TOPIC=agg-to-mape
ENV ACTUATOR_TOPIC_PREFIX=zone.commands.
ENV LEDGER_TOPIC_PREFIX=zone.ledger.
ENV LEDGER_MAPE_PARTITION=1
ENV PROPERTIES_PATH=/app/config/mape.properties
ENV POLL_INTERVAL_MS=250
ENV ACTUATOR_PARTITIONS=3
ENV TOPIC_REPLICATION=1
EXPOSE 8080
ENTRYPOINT ["/app/mape"]
