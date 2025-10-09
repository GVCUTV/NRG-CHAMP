# v3
# file: README.md
# NRG CHAMP Gamification Service — Leaderboard MVP

## Purpose and MVP scope

The gamification service consumes finalized epochs from the public ledger and
projects them into a single, global leaderboard. It currently delivers the
minimum viable experience required by the platform:

- Rankings are computed for the trailing **24-hour** and **7-day** windows. Any
  other window requested by clients is ignored in favour of these canonical
  spans, and `24h` is served when no query parameter is supplied.
- Scores track only **energy consumption in kilowatt-hours**. Each zone’s
  kWh total is the sole metric considered when ordering participants.
- Entries are ordered by ascending consumption so the lowest energy usage wins.
  Ranks are assigned sequentially without tie handling; when two zones report
  the same kWh total they still receive distinct rank numbers.
- The HTTP surface always exposes a single **global** scope. Per-building or
  per-floor breakdowns are intentionally out of scope for this MVP.

## Ledger input contract (v1)

Gamification subscribes to the `epoch.public` stream emitted by the ledger and
currently understands only schema version `v1`. Any other type or schema is
rejected before scoring.

### Accepted payload shape

- `type` — must equal `"epoch.public"`.
- `schemaVersion` — must equal `"v1"`.
- `zoneId` — required zone identifier. Empty values are logged and ignored.
- `epochIndex` — optional sequential index. When present it is cached alongside
  the score for observability only.
- `matchedAt` — required RFC3339 timestamp. Leaderboard windowing relies solely
  on this field; payloads missing the field are dropped.
- `aggregator.summary.zoneEnergyKWhEpoch` — required numeric kilowatt-hour
  consumption for the epoch. If absent the event is counted as missing energy
  and excluded from rankings.

Example payload consumed successfully by the decoder:

```json
{
  "type": "epoch.public",
  "schemaVersion": "v1",
  "zoneId": "zone-001",
  "epochIndex": 7,
  "matchedAt": "2024-05-02T15:04:05Z",
  "aggregator": {
    "summary": {
      "zoneEnergyKWhEpoch": 42.5
    }
  },
  "energyKWh_total": 99.1,
  "epoch": "2024-05-02T15:00:00Z"
}
```

Field usage inside Gamification:

- `zoneId` selects the leaderboard entry to update.
- `matchedAt` anchors the reading within the rolling windows described below.
- `aggregator.summary.zoneEnergyKWhEpoch` is added to the running totals that
  drive rank ordering.
- `epochIndex` and legacy totals such as `energyKWh_total` are stored only for
  diagnostics; they do not influence the standings.

## Leaderboard computation

- **Windowing** — Windows are derived exclusively from the `matchedAt` timestamp
  of each accepted payload. The service maintains trailing windows of **24
  hours** and **7 days**. Other durations in requests are ignored and `24h` is
  returned.
- **Ranking** — For every window the service sums `zoneEnergyKWhEpoch` per zone
  and sorts the results in ascending order so the lowest consumption ranks
  first. No tie-breakers are applied; identical totals keep their insertion
  order and still receive distinct rank integers.

## Troubleshooting ledger ingestion

Decode outcomes are exported under `gamification_ledger_decode_drop_total` with a
`reason` label. The most common values are:

| Reason label | Meaning | Follow-up |
| --- | --- | --- |
| `missing_matchedAt` | Payload omitted the required `matchedAt` field. The event cannot be windowed and is dropped. | Inspect upstream ledger publisher; ensure it stamps the match timestamp. |
| `json_error` | JSON could not be parsed or contained incompatible types (e.g. string instead of number for `zoneEnergyKWhEpoch`). | Verify the producer schema, then replay or repair the offending record. |
| `schema_reject` | `type` or `schemaVersion` did not normalize to `epoch.public`/`v1`, or the value is not listed in `LEDGER_SCHEMA_ACCEPT`. | Update the allow-list or ensure the publisher still emits v1 payloads. |

Additional counters:

- `gamification_ledger_decode_ok_total` — number of successfully decoded
  payloads after schema checks.
- `gamification_ledger_energy_missing_total` — epochs that passed validation but
  lacked `aggregator.summary.zoneEnergyKWhEpoch`. These entries are excluded
  from standings until upstream fills the metric.

## Configuration

Runtime configuration layers defaults, the optional properties file, and finally
environment variables. Keys not listed below are ignored.

### Core service settings

- `GAMIFICATION_PROPERTIES_PATH` (env, default `gamification.properties`):
  location of the layered configuration file bundled with the container.
- The properties file recognises the following keys. Each entry can be replaced
  via the matching environment override listed in the last column.

| Property | Default | Override env |
| --- | ------- | ------------- |
| `listen_address` | `:8086` | `GAMIFICATION_LISTEN_ADDRESS` |
| `log_path` | `logs/gamification.log` | `GAMIFICATION_LOG_PATH` |
| `http_read_timeout_ms` | `5000` | `GAMIFICATION_HTTP_READ_TIMEOUT_MS` |
| `http_write_timeout_ms` | `10000` | `GAMIFICATION_HTTP_WRITE_TIMEOUT_MS` |
| `shutdown_timeout_ms` | `5000` | `GAMIFICATION_SHUTDOWN_TIMEOUT_MS` |
| `kafka_brokers` | `kafka:9092` | `GAMIFICATION_KAFKA_BROKERS` or `KAFKA_BROKERS` |
| `ledger_topic` | `ledger.public.epochs` | `GAMIFICATION_LEDGER_TOPIC` or `LEDGER_TOPIC` |
| `ledger_group_id` | `gamification-ledger` | `GAMIFICATION_LEDGER_GROUP` |
| `ledger_poll_timeout_ms` | `5000` | `GAMIFICATION_LEDGER_POLL_TIMEOUT_MS` |
| `max_epochs_per_zone` | `1000` | `GAMIF_MAX_EPOCHS_PER_ZONE` |

Additional guardrails:

- `LEDGER_SCHEMA_ACCEPT` (env, default `v1,legacy`): comma-separated list of
  ledger schema identifiers accepted by the consumer. Messages outside this
  allowlist are dropped and counted via telemetry.

When running through Docker Compose, update `listen_address` (or set
`GAMIFICATION_LISTEN_ADDRESS`) to `:8085` so the container port matches the
published mapping.

### HTTP overlay settings

| Key | Default | Notes |
| --- | ------- | ----- |
| `GAMIF_HTTP_PORT` | `8085` | Logged for observability; the listener address still derives from `listen_address`. |
| `GAMIF_WINDOWS` | `24h,7d` | Comma-separated list of requested leaderboard windows. Only `24h` and `7d` are honoured; other values are ignored. |
| `GAMIF_REFRESH_EVERY` | `60s` | Interval between background leaderboard refreshes. |

## Running with Docker Compose

The repository root ships a Compose definition that builds the image with the
Go workspace and shared circuit breaker module already wired in.

1. From the repository root, ensure the supporting services are up:
   ```bash
   docker compose up -d kafka ledger
   ```
2. Start the gamification service:
   ```bash
   docker compose up -d gamification
   ```
   The container exposes port `8085` by default; either update the properties
   file or set `GAMIFICATION_LISTEN_ADDRESS=:8085` to align the bind address with
   the published port.
3. Follow logs as needed with `docker compose logs -f gamification`.

## API reference

### `GET /leaderboard`

Returns the latest in-memory leaderboard snapshot.

- **Query parameters**
  - `window` (optional): accepts `24h` or `7d`. Invalid values fall back to
    `24h`.
- **Response** (`200 OK`, `application/json`)
  ```json
  {
    "generatedAt": "2024-05-10T12:00:00Z",
    "scope": "global",
    "window": "24h",
    "entries": [
      { "rank": 1, "zoneId": "zone-a", "energyKWh": 123.45 },
      { "rank": 2, "zoneId": "zone-b", "energyKWh": 150.12 }
    ]
  }
  ```
  - `generatedAt`: UTC timestamp for the snapshot creation.
  - `scope`: always `global` in the current MVP.
  - `window`: resolved aggregation window served for the request.
  - `entries`: ordered ascending by `energyKWh`; ranks increment by one even
    when kWh totals match.

Health probes remain available at `/health`, `/health/live`, and
`/health/ready`, and Prometheus metrics render under `/metrics`.

## Operational notes

- Kafka reads are protected by the shared circuit breaker. When the breaker
  opens the consumer backs off from Kafka, but the HTTP server keeps serving the
  most recent snapshot held in memory.
- Leaderboards are refreshed on a fixed cadence (`GAMIF_REFRESH_EVERY`) and use
  the bounded per-zone buffer governed by `max_epochs_per_zone` to keep memory
  usage predictable.
- Because only kWh totals are tracked, client UIs can treat the payload as an
  ordered list without additional tie-breaking metadata.
