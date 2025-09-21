# Room Simulator (merged sensors+actuators -> Kafka)
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY sim.properties ./
RUN CGO_ENABLED=0 GOOS=linux go build -o zone-simulator ./

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -H appuser
COPY --from=builder /app/zone-simulator /app/zone-simulator
COPY --from=builder /app/sim.properties /app/sim.properties
ENV SIM_PROPERTIES=/app/sim.properties
# Optional defaults
ENV KAFKA_BROKERS=kafka:9092
ENV TOPIC_PREFIX=device.readings
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=2s --retries=5 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/zone-simulator"]
