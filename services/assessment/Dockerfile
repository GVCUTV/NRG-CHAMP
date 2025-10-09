// v1
// Dockerfile
# Build stage
FROM golang:1.22 AS build
WORKDIR /app
COPY go.work go.work.sum ./
COPY services/assessment/go.mod services/assessment/go.sum ./services/assessment/
RUN --mount=type=cache,target=/go/pkg/mod \
    go -C services/assessment mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go -C services/assessment build -o /out/assessment ./cmd/assessment

# Runtime stage
FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/assessment /assessment
EXPOSE 8085
ENV ASSESSMENT_BIND_ADDR=":8085"
ENTRYPOINT ["/assessment"]
