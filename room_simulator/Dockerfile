# Room Simulator (merged sensors+actuators -> Kafka)
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o room-simulator ./cmd/room-simulator

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -H appuser
COPY --from=builder /app/room-simulator /app/room-simulator
COPY sim.properties /app/sim.properties
ENV SIM_PROPERTIES=/app/sim.properties
# Optional defaults
ENV KAFKA_BROKERS=kafka:9092
ENV TOPIC_PREFIX=device.readings
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=2s --retries=5 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/room-simulator"]
