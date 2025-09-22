# v0
# Makefile
APP=mape

build:
	CGO_ENABLED=0 go build -o bin/$(APP) ./cmd/mape

run:
	LOG_DIR=./logs KAFKA_BROKERS=localhost:9092 go run ./cmd/mape

docker:
	docker build -t $(APP):latest .

fmt:
	go fmt ./...
