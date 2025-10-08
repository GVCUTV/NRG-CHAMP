// v1
// services/mape/README.md
# v1
# README.md

## MAPE Service (NRG CHAMP)

This service consumes **Aggregator → MAPE** readings from Kafka (one partition per zone),
performs **Analyze/Plan** vs targets and hysteresis defined in `configs/mape.properties`,
and **Execute** by publishing **MAPE → Actuators** commands. It also writes a **MAPE ledger event**
to the per-zone ledger topic at partition **1** (partition 0 is reserved to Aggregator).

### Topics layout

- `AGGREGATOR_TOPIC` (default: `aggregator.to.mape`): **N partitions = number of zones**.
  - **Partition i ↔ zones[i]** in the `zones` list inside `mape.properties` (order matters).
  - The MAPE drains all available records on a partition but **uses only the most recent**.
- `ACTUATOR_TOPIC_PREFIX` (default: `zone.commands.`): per-zone topic, e.g. `zone.commands.zone-a`.
  - Partitions are **actuators** (hash-balanced by `actuatorId` key).
- `LEDGER_TOPIC_PREFIX` (default: `zone.ledger.`): per-zone topic with **2 partitions**.
  - Partition 0: Aggregator; Partition **1: MAPE**.

### Build & Run

```bash
# build
docker build -t mape:latest .

# run (example)
docker run --rm -p 8080:8080 \
  -e KAFKA_BROKERS=broker:9092 \
  -e AGGREGATOR_TOPIC=aggregator.to.mape \
  -e ACTUATOR_TOPIC_PREFIX=zone.commands. \
  -e LEDGER_TOPIC_PREFIX=zone.ledger. \
  -e LEDGER_MAPE_PARTITION=1 \
  -v $(pwd)/configs:/app/configs \
  mape:latest
```

### HTTP API

- `GET /health` → `200 OK`
- `GET /status` → JSON of loop/message counters
- `POST /config/reload` → reloads `mape.properties` **and** reapplies defaults to the runtime setpoint store.
- `GET /config/temperature` → returns `{ "setpoints": { "zone-A": 22.0, ... } }`.
- `GET /config/temperature/{zoneId}` → returns `{ "zoneId": "zone-A", "setpointC": 22.0 }`.
- `PUT /config/temperature/{zoneId}` with `{ "setpointC": 23.5 }` updates the in-memory setpoint (validated within `MAPE_SETPOINT_MIN_C..MAPE_SETPOINT_MAX_C`).

Runtime updates are **not persisted** to `mape.properties`; restarting the service restores the file-based defaults.

### Temperature setpoints

- `mape.properties` lists zones via `zones=zone-A,zone-B,...` and a global default `target=<float>`.
- Optional overrides: `target.zone-A=23.0` applies only to `zone-A`; zones without overrides inherit the global `target`.
- Validation defaults: `MAPE_SETPOINT_MIN_C=10.0`, `MAPE_SETPOINT_MAX_C=35.0` (overridable via env vars).

Setpoints are cached in a thread-safe store so that Analyze/Plan reads the latest values while HTTP requests mutate them.

### Notes on Dependencies

Kafka access uses `github.com/segmentio/kafka-go` (minimal, well‑maintained) wrapped by the shared
`github.com/nrg-champ/circuitbreaker` module so outages trigger the circuit breaker before
propagating. All other code relies only on the standard library. Logging is done via `slog`
to **both** a logfile and stdout, as required.


---
## v2 — K8s base/overlays
- `k8s/base` → set minimale (namespace, configmap, deployment, service).
- `k8s/overlays/dev` → base + Secret broker (semplice).
- `k8s/overlays/prod` → base + SA/RBAC + NetworkPolicy + PDB + resources + Secret.

### Apply
```bash
# Dev
kubectl apply -k k8s/overlays/dev

# Prod
kubectl apply -k k8s/overlays/prod
```


### v3 — Topic auto-creation
- Il servizio tenta di **creare/validare** i topic necessari all'avvio (idempotente):
  - `AGGREGATOR_TOPIC` → partizioni = numero di zone.
  - `ACTUATOR_TOPIC_PREFIX + <zone>` → partizioni = `ACTUATOR_PARTITIONS` (default 2).
  - `LEDGER_TOPIC_PREFIX + <zone>` → partizioni = 2.
- Nuove env:
  - `ACTUATOR_PARTITIONS` (default: 2)
  - `TOPIC_REPLICATION` (default: 1)
