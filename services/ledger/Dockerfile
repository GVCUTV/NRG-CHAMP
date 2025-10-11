# // v4
# // file: services/ledger/Dockerfile
FROM golang:1.23-alpine AS ledger_build
WORKDIR /src
COPY services/ledger/go.mod services/ledger/go.sum ./
COPY circuit_breaker /circuit_breaker
COPY services/ledger/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p /out && \
    go build -trimpath -ldflags="-s -w" -o /out/ledger .

FROM alpine:3.20 AS ledger_runtime
WORKDIR /app
COPY --from=ledger_build /out/ledger /usr/local/bin/ledger
RUN adduser -D -H -u 10001 ledger && mkdir -p /app/data /app/logs && chown -R ledger:ledger /app
USER ledger
EXPOSE 8083
ENTRYPOINT ["/usr/local/bin/ledger"]
