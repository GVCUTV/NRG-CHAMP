# // v3
# // file: services/gamification/Dockerfile
FROM golang:1.23-alpine AS gamification_build_stage
WORKDIR /src
COPY circuit_breaker /circuit_breaker
COPY services/gamification/go.mod services/gamification/go.sum ./services/gamification/
COPY services/gamification/ ./services/gamification/
RUN --mount=type=cache,target=/go/pkg/mod \
    cd services/gamification && go mod tidy && go mod download
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd services/gamification/cmd/gamification && go build -o /out/gamification

FROM alpine:3.20 AS gamification_runtime_stage
WORKDIR /srv
RUN adduser -D -H appuser
COPY --from=gamification_build_stage /out/gamification /usr/local/bin/gamification
ENV BROKER_ADDRESS=kafka:9092 \
    LEDGER_TOPIC=ledger.public.epochs \
    LEDGER_CONSUMER_GROUP=gamification-ledger \
    GAMIFICATION_PORT=8085 \
    GAMIFICATION_LOG_PATH=/srv/logs/gamification.log \
    GAMIFICATION_HTTP_READ_TIMEOUT_MS=5000 \
    GAMIFICATION_HTTP_WRITE_TIMEOUT_MS=10000 \
    GAMIFICATION_SHUTDOWN_TIMEOUT_MS=5000 \
    LEDGER_POLL_TIMEOUT_MS=5000 \
    MAX_EPOCHS_PER_ZONE=1000
USER appuser
EXPOSE 8085
ENTRYPOINT ["/usr/local/bin/gamification"]
