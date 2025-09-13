// v0
// Dockerfile
# Build stage
FROM golang:1.22 AS build
WORKDIR /app
COPY . ./
RUN --mount=type=cache,target=/go/pkg/mod     --mount=type=cache,target=/root/.cache/go-build     cd assessment && go build -o /out/assessment ./cmd/assessment

# Runtime stage
FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/assessment /assessment
EXPOSE 8085
ENV ASSESSMENT_BIND_ADDR=":8085"
ENTRYPOINT ["/assessment"]
