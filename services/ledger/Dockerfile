# // v7
# // file: services/ledger/Dockerfile
FROM golang:1.23-alpine AS ledger_build
RUN apk add --no-cache git ca-certificates
WORKDIR /src
COPY go.work go.work.sum ./
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    go -C services/ledger mod download
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go -C services/ledger build -o /out/ledger .

FROM alpine:3.20 AS ledger_runtime
WORKDIR /app
COPY --from=ledger_build /out/ledger /usr/local/bin/ledger
RUN adduser -D -H -u 10001 ledger && mkdir -p /app/data /app/logs && chown -R ledger:ledger /app
USER ledger
EXPOSE 8083
ENTRYPOINT ["/usr/local/bin/ledger"]
