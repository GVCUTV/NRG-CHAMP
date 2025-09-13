# Gamification Service (NRG CHAMP)

**Status:** v0 — 2025-09-13T09:58:52.869545Z

This service computes **scores** and exposes **leaderboards** based *strictly on the Ledger service* (HTTP read‑only).
It is **independent** from the Assessment service.

## APIs

- `POST /score/recompute?zoneId=&from=&to=` — Recompute scores deterministically from the Ledger for a time window.
- `GET /leaderboard?scope=building|floor|zone&page=&size=` — Paged, sorted leaderboard.
- `GET /health` — Liveness.

## Configuration

Environment variables (also provided via Kubernetes ConfigMap):

- `LEDGER_BASE_URL` (default: `http://ledger:8084`) — Base URL of the Ledger HTTP API.
- `WEIGHT_COMFORT` (default: `1.0`)
- `WEIGHT_ANOMALY` (default: `-5.0`)
- `WEIGHT_ENERGY` (default: `-0.1`)
- `ZONE_ID_DELIM` (default: `:`) — Delimiter for parsing `building:floor:zone` grouping.
- `DATA_DIR` (default: `/data`) — Where the store writes `scores.json` and log files.
- `LOG_LEVEL` (default: `INFO`)

## Files written

- `{DATA_DIR}/scores.json` — persistent score store (append‑safe JSON file).
- `{DATA_DIR}/gamification.log` — combined plain‑text log.

## Build & Run (local)

```bash
cd gamification
go build ./cmd/gamification
LEDGER_BASE_URL=http://localhost:8084 ./gamification
```

## Docker

```bash
docker build -t nrgchamp/gamification:latest ./gamification
docker run --rm -p 8086:8086 -e LEDGER_BASE_URL=http://host.docker.internal:8084 -v $PWD/data:/data nrgchamp/gamification:latest
```

## Docker Compose (fragment)

A minimal compose file is provided at the repo root: `docker-compose.gamification.yaml`.
Merge its `services.gamification` section into your main Compose file, or run it standalone:

```bash
docker compose -f docker-compose.gamification.yaml up --build
```

## Kubernetes

Manifests live under `k8s/gamification`. Apply with:

```bash
kubectl apply -k k8s/gamification
```

> Ensure the `LEDGER_BASE_URL` in the ConfigMap points to the in‑cluster Ledger service DNS name (defaults to `http://ledger:8084`).

