# // v4
# // file: services/gamification/Dockerfile
# v3.1 — module-centric, correct circuit_breaker location
FROM golang:1.23-alpine AS gamification_build
WORKDIR /src

# Selective COPY: service + local lib to /src/circuit_breaker (NOT /circuit_breaker)
COPY services/gamification/go.mod services/gamification/go.sum ./services/gamification/
COPY circuit_breaker/ ./circuit_breaker/
COPY services/gamification/ ./services/gamification/

# Workspace off so go.work cannot conflict
ENV GOWORK=off

# Download deps and build from the module dir
RUN --mount=type=cache,target=/go/pkg/mod \
    go -C services/gamification mod download
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go -C services/gamification build -o /out/gamification ./cmd/gamification

FROM alpine:3.20 AS gamification_runtime
WORKDIR /srv
RUN adduser -D -H appuser \
 && mkdir -p /var/log/gamification \
 && chown appuser:appuser /var/log/gamification
COPY --from=gamification_build /out/gamification /srv/gamification
USER appuser
EXPOSE 8085
ENV GAMIFICATION_BIND_ADDR=":8085"
ENTRYPOINT ["/srv/gamification"]
