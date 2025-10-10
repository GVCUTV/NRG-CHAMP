# v2
# Dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p /out && \
    go build -trimpath -ldflags="-s -w" -o /out/ledger .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/ledger /usr/local/bin/ledger
RUN adduser -D -H -u 10001 ledger && mkdir -p /app/data /app/logs && chown -R ledger:ledger /app
USER ledger
EXPOSE 8083
ENTRYPOINT ["/usr/local/bin/ledger"]