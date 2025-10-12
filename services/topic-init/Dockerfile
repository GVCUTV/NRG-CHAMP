# // v4
# // file: services/topic-init/Dockerfile
FROM golang:1.23-alpine AS topic_init_builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src
COPY go.work go.work.sum ./
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    go -C services/topic-init mod download
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go -C services/topic-init build -o /out/topic-init ./cmd/topic-init

FROM alpine:3.20 AS topic_init_runtime
WORKDIR /app
RUN adduser -D -H -u 10001 appuser
COPY --from=topic_init_builder /out/topic-init /app/topic-init
USER appuser
ENTRYPOINT ["/app/topic-init"]
