# v0
# Dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o mape ./cmd/mape

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -H appuser
COPY --from=build /app/mape /app/mape
COPY --from=build /app/mape.properties /app/mape.properties
ENV HTTP_BIND=:8080
ENV LOG_DIR=/app/logs
ENV KAFKA_BROKERS=localhost:9092
ENV AGGREGATOR_TOPIC=agg-to-mape
ENV ACTUATOR_TOPIC_PREFIX=zone.commands.
ENV LEDGER_TOPIC_PREFIX=zone.ledger.
ENV LEDGER_MAPE_PARTITION=1
ENV PROPERTIES_PATH=/app/mape.properties
ENV POLL_INTERVAL_MS=5000
ENV ACTUATOR_PARTITIONS=2
ENV TOPIC_REPLICATION=1
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=2s --retries=5 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/mape"]
