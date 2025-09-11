// v2
// README.md
# Ledger Service (NRG CHAMP) — Standalone

**Independent** from `room_simulator`. Provides append-only hash-chain ledger with REST API, Docker, and K8s.

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
