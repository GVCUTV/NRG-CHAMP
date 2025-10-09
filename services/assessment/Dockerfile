# v4 — module-centric, build from module dir, correct local lib path
FROM golang:1.23-alpine AS assessment_build
WORKDIR /src

# Copy only what's needed; place circuit_breaker under /src/circuit_breaker
COPY services/assessment/go.mod services/assessment/go.sum ./services/assessment/
COPY circuit_breaker/ ./circuit_breaker/
COPY services/assessment/ ./services/assessment/

# Disable workspace so go.work is ignored inside the container
ENV GOWORK=off

# Fetch deps and build from INSIDE the module directory
RUN --mount=type=cache,target=/go/pkg/mod \
    go -C services/assessment mod download
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go -C services/assessment build -o /out/assessment ./cmd/assessment

# Minimal runtime
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
