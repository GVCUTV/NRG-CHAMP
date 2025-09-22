# v3
# file: Dockerfile
FROM golang:1.22 AS build
WORKDIR /app
COPY . /app/
# build aggregator
RUN cd aggregator && go mod tidy && CGO_ENABLED=0 go build -o /bin/aggregator ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY --from=build /bin/aggregator /usr/local/bin/aggregator
COPY aggregator/aggregator.properties /app/aggregator.properties
COPY aggregator/data /app/data
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/aggregator","-props","/app/aggregator.properties"]
