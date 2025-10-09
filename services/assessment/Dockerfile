// v3
// Dockerfile
FROM golang:1.23-alpine AS assessment_build
RUN apk add --no-cache git ca-certificates
WORKDIR /src
COPY go.work go.work.sum ./
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    go -C services/assessment mod download
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go -C services/assessment build -o /out/assessment ./cmd/assessment

FROM alpine:3.20 AS assessment_runtime
WORKDIR /app
RUN adduser -D -H appuser
COPY --from=assessment_build /out/assessment /app/assessment
EXPOSE 8085
ENV ASSESSMENT_BIND_ADDR=":8085"
USER appuser
ENTRYPOINT ["/app/assessment"]
