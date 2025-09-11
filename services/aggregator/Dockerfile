# v1
# file: Dockerfile
# Build a minimal static Go image
FROM golang:1.22 AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /aggregator ./main.go

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /aggregator /aggregator
ENV AGG_LISTEN_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/aggregator"]
