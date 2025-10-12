# // v2
# // file: services/topic-init/Dockerfile
FROM golang:1.23-alpine AS topic_init_builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY services/topic-init/go.mod services/topic-init/go.sum ./services/topic-init/
RUN --mount=type=cache,target=/go/pkg/mod go -C services/topic-init mod download
COPY . .
RUN CGO_ENABLED=0 go -C services/topic-init build -o /topic-init ./cmd/topic-init

FROM alpine:3.20 AS topic_init_runtime
WORKDIR /app
RUN adduser -D -H -u 10001 appuser
COPY --from=topic_init_builder /topic-init /app/topic-init
USER appuser
ENTRYPOINT ["/app/topic-init"]
