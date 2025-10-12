# // v2
# // file: services/gamification/Dockerfile
FROM golang:1.23-alpine AS gamification_build_stage
WORKDIR /src
COPY circuit_breaker /circuit_breaker
COPY services/gamification/go.mod services/gamification/go.sum ./services/gamification/
COPY services/gamification/ ./services/gamification/
RUN --mount=type=cache,target=/go/pkg/mod \
    cd services/gamification && go mod tidy && go mod download
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd services/gamification/cmd/gamification && go build -o /out/gamification

FROM alpine:3.20 AS gamification_runtime_stage
WORKDIR /srv
RUN adduser -D -H appuser
COPY --from=gamification_build_stage /out/gamification /usr/local/bin/gamification
COPY services/gamification/gamification.properties /srv/gamification.properties
ENV GAMIFICATION_PROPERTIES_PATH=/srv/gamification.properties
USER appuser
EXPOSE 8085
ENTRYPOINT ["/usr/local/bin/gamification"]
