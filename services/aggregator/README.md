# Aggregator Service

Consumes readings from Kafka topic `device.readings` (via sidecar), validates schema,
buffers the last 15 minutes in memory, and exposes:

- `GET /health`
- `GET /latest?zoneId=Z`
- `GET /series?zoneId=Z&from=RFC3339&to=RFC3339`
- `POST /ingest` (internal, used by the kcat sidecar; accepts NDJSON or JSON array)

## Local run
```bash
go run ./main.go
```

## Env vars
- `AGG_LISTEN_ADDR` (default `:8080`)
- `AGG_BUFFER_MINUTES` (default `15`)
```