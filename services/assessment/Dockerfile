// v2
// services/assessment/Dockerfile
# Build stage
FROM golang:1.23-alpine AS assessment_build
WORKDIR /src
COPY . .
COPY circuit_breaker /circuit_breaker
ENV GOWORK=off
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    sh -c 'mkdir -p /out && CGO_ENABLED=0 go build -o /out/assessment ./services/assessment/cmd/assessment'

# Runtime stage
FROM alpine:3.20 AS assessment_runtime
WORKDIR /
RUN adduser -D -H assessment \
 && mkdir -p /var/log/assessment \
 && chown assessment:assessment /var/log/assessment
COPY --from=assessment_build /out/assessment /assessment
USER assessment
EXPOSE 8085
ENV ASSESSMENT_BIND_ADDR=":8085"
ENTRYPOINT ["/assessment"]
