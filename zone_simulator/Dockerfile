# // v2
# // file: zone_simulator/Dockerfile
# Room Simulator (merged sensors+actuators -> Kafka)
FROM golang:1.23-alpine AS zs_builder_base
WORKDIR /app
FROM golang:1.23-alpine AS zs_builder
WORKDIR /app
COPY zone_simulator/go.mod zone_simulator/go.sum ./
COPY circuit_breaker /circuit_breaker
COPY zone_simulator/*.go ./
COPY zone_simulator/sim.properties ./
# Popola correttamente go.sum (inclusi transitivi di circuit_breaker)
RUN --mount=type=cache,target=/go/pkg/mod go mod download && go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o zone-simulator ./

FROM alpine:3.20 AS zs_runtime
WORKDIR /app
RUN adduser -D -H appuser
COPY --from=zs_builder /app/zone-simulator /app/zone-simulator
COPY --from=zs_builder /app/sim.properties /app/sim.properties
ENV SIM_PROPERTIES=/app/sim.properties
# Optional defaults
ENV KAFKA_BROKERS=kafka:9092
ENV TOPIC_READINGS_PREFIX=device.readings
ENV TOPIC_COMMANDS_PREFIX=zone.commands
ENV CB_ENABLED=false
ENV CB_KAFKA_FAILURE_THRESHOLD=5
ENV CB_KAFKA_SUCCESS_THRESHOLD=2
ENV CB_KAFKA_OPEN_SECONDS=30
ENV CB_KAFKA_TIMEOUT_MS=3000
ENV CB_KAFKA_BACKOFF_MS=200
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=2s --retries=5 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/zone-simulator"]
