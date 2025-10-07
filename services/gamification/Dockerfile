# v0
# Dockerfile
# Build stage
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY . ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd cmd/gamification && go build -o /out/gamification

# Runtime stage
FROM alpine:3.20
RUN adduser -D -H -u 10001 appuser
WORKDIR /srv
COPY --from=build /out/gamification /usr/local/bin/gamification
ENV DATA_DIR=/data \
    LISTEN_ADDRESS=:8086 \
    LEDGER_BASE_URL=http://ledger:8084
RUN mkdir -p /data && chown -R appuser /data
USER appuser
EXPOSE 8086
ENTRYPOINT ["/usr/local/bin/gamification"]
