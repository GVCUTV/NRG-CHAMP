# v0
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
- `POST /config/reload` → reloads `mape.properties`

### Notes on Dependencies

Kafka access uses `github.com/segmentio/kafka-go` (minimal, well‑maintained). All other
code relies only on the standard library. Logging is done via `slog` to **both** a logfile
and stdout, as required.
