# // v5
# // file: services/ledger/Dockerfile
# v3.1 — module-centric, auto-detect main
FROM golang:1.23-alpine AS ledger_build
WORKDIR /src

# Selective COPY: service + local lib to /src/circuit_breaker
COPY services/ledger/go.mod services/ledger/go.sum ./services/ledger/
COPY circuit_breaker/ ./circuit_breaker/
COPY services/ledger/ ./services/ledger/

# Workspace off to avoid go.work interference
ENV GOWORK=off

# Download deps from module dir
RUN --mount=type=cache,target=/go/pkg/mod \
    go -C services/ledger mod download

# Auto-detect the main package path and build
RUN --mount=type=cache,target=/root/.cache/go-build sh -euxc '\
  PKG=./cmd/ledger; \
  if [ ! -d "services/ledger/cmd/ledger" ]; then \
    if [ -d "services/ledger/cmd/server" ]; then PKG=./cmd/server; \
    else PKG=./; fi; \
  fi; \
  CGO_ENABLED=0 go -C services/ledger build -trimpath -ldflags="-s -w" -o /out/ledger "$PKG" \
'

FROM alpine:3.20 AS ledger_runtime
WORKDIR /
RUN adduser -D -H ledger \
 && mkdir -p /var/log/ledger \
 && chown ledger:ledger /var/log/ledger
COPY --from=ledger_build /out/ledger /usr/local/bin/ledger
USER ledger
EXPOSE 8084
ENV LEDGER_BIND_ADDR=":8084"
ENTRYPOINT ["/usr/local/bin/ledger"]
