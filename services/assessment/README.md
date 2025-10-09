// v1
// README.md
# Assessment Service (Service 5)

**Purpose**: Compute KPIs strictly from the **Ledger** service and expose them via HTTP.

## KPI Specification (authoritative)
- **comfort_time_pct** — `100 × Σ overlap_i × 1(|temp_i − target| ≤ tol) / Σ overlap_i`. Overlaps use the intersection between
the sample hold interval and the requested window. Bounds are inclusive, tolerance `< 0` or `NaN` is clamped to `0 °C`, and
`target = NaN` yields `0`.
- **mean_dev** — `Σ overlap_i × |temp_i − target| / Σ overlap_i` (°C) using the same overlaps as comfort. Empty/missing data or
`target = NaN` return `0`.
- **actuator_on_pct** — `100 × overlap(ON intervals, window) / window_length`. ON intervals derive from Ledger action events with
the last known state before the window. A zero-length window returns `0`.
- **anomaly_count** — number of anomaly events with timestamps in `[from, to)`.

These formulas are implemented in `internal/kpi/compute.go` and cross-referenced from
[docs/project_documentation.md](../docs/project_documentation.md).

## KPIs
- `comfort_time_pct`: percentage of time within tolerance of target temperature.
- `anomaly_count`: count of anomaly events.
- `mean_dev`: mean absolute deviation from target temperature (°C).
- `actuator_on_pct`: percentage of time at least one actuator is ON.

## API
- `GET /health`
- `GET /kpi/summary?zoneId=&from=&to=`
- `GET /kpi/series?metric=&zoneId=&from=&to=&bucket=`

Times are RFC3339. If `from/to` omitted: last hour until now. Default bucket: 5m.

## Environment
- `LEDGER_BASE_URL` (default `http://ledger:8084`)
- `TARGET_TEMP_C` (default `22`)
- `COMFORT_TOLERANCE_C` (default `0.5`)
- `CACHE_TTL` (default `30s`)
- `ASSESSMENT_BIND_ADDR` (default `:8085`)
- `ASSESSMENT_LOGFILE` (default `./assessment.log`)

## Run locally
```bash
cd assessment
go run ./cmd/assessment
```

## Docker
```bash
docker build -t nrgchamp/assessment:local -f Dockerfile .
docker run --rm -p 8085:8085 -e LEDGER_BASE_URL=http://host.docker.internal:8084 nrgchamp/assessment:local
```

## Docker Compose
A compose fragment is provided at `assessment/deploy/docker-compose.assessment.yml`. You can merge it into your root `docker-compose.yml` or include with:
```bash
docker compose -f docker-compose.yml -f assessment/deploy/docker-compose.assessment.yml up -d --build
```

## Kubernetes
```bash
kubectl apply -k assessment/k8s/assessment
```

## Logging
All operations are logged to stdout and to the logfile via `log/slog`.
