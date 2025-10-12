# // v1
# // file: services/topic-init/Dockerfile
FROM golang:1.23-alpine AS topic_init_builder
WORKDIR /src
ENV GOWORK=off

COPY services/topic-init/go.mod services/topic-init/go.sum ./services/topic-init/
RUN --mount=type=cache,target=/go/pkg/mod go -C services/topic-init mod download
COPY circuit_breaker ./circuit_breaker
COPY services/topic-init/ ./services/topic-init/
RUN mkdir -p /out \
    && CGO_ENABLED=0 go -C services/topic-init build -o /out/topic-init ./cmd/topic-init

FROM alpine:3.20 AS topic_init_runtime
WORKDIR /app
RUN adduser -D -H -u 10001 appuser
COPY --from=topic_init_builder /out/topic-init /app/topic-init
USER appuser
ENTRYPOINT ["/app/topic-init"]
