// v1
// Dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.22 as builder
WORKDIR /src
COPY . ./
RUN --mount=type=cache,target=/root/.cache/go-build \    --mount=type=cache,target=/go/pkg/mod \    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/mape ./cmd/server

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /out/mape /app/mape
USER 65532:65532
ENV LISTEN_ADDR=:8090
EXPOSE 8090
ENTRYPOINT ["/app/mape"]
