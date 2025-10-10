// v3
// README.md
# Ledger Service (NRG CHAMP) — Standalone

**Independent** from `room_simulator`. Provides append-only hash-chain ledger with REST API and Kafka ingestion for epoch-matched ledger entries.

## Configuration

The service reads configuration from flags or environment variables. Key variables:

| Variable | Description | Default |
| --- | --- | --- |
| `LEDGER_ADDR` | HTTP listen address | `:8083` |
| `LEDGER_DATA` | Directory for ledger data file | `./data` |
| `LEDGER_LOGS` | Directory for log files | `./logs` |
| `LEDGER_KAFKA_BROKERS` | Comma-separated Kafka broker list | `kafka:9092` |
| `LEDGER_GROUP_ID` | Consumer group identifier | `ledger-service` |
| `LEDGER_TOPIC_TEMPLATE` | Topic template containing `{zone}` placeholder | `ledger-{zone}` |
| `LEDGER_ZONES` | Comma-separated list of zones to monitor | _(required)_ |
| `LEDGER_EPOCH_GRACE_MS` | Milliseconds to wait before imputing missing counterparts | `2000` |
| `LEDGER_BUFFER_MAX_EPOCHS` | Number of finalized epochs to retain for deduplication | `200` |

Partition assignments follow the documented convention: partition `0` carries Aggregator payloads, partition `1` carries MAPE payloads.

## Run (Go)
```bash
cd ledger
go run ./... -addr :8083 -data ./data -logs ./logs
```

## Docker
```bash
cd ledger
docker compose up --build -d
```

## Kubernetes
```bash
kubectl apply -k ledger/k8s
```
