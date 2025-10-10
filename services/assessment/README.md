// v1
// README.md
# Assessment Service (Service 5)

**Purpose**: Compute KPIs strictly from the **Ledger** service and expose them via HTTP.

## KPIs
Both `GET /kpi/summary` and `GET /kpi/series` return the KPIs defined below.

### KPI definitions
- `comfort_time_pct`: Time-weighted percentage of the window `[from, to)` during which the absolute temperature error `|temp - target|` is less than or equal to the tolerance. Weighting follows the duration between consecutive readings within the window, so intervals without telemetry do not contribute to the numerator or denominator.
- `mean_dev`: Sample-weighted mean absolute deviation, in °C, between each temperature reading observed in `[from, to)` and the target temperature. Every reading contributes equally regardless of spacing.
- `actuator_on_pct`: Percentage of the window duration, in seconds, where at least one actuator is ON. Computed as the overlap between ON intervals and `[from, to)` divided by the window length (falls back to one second if `to <= from` to avoid division by zero).
- `anomaly_count`: Count of anomaly events supplied by the Ledger with timestamps in `[from, to)`. The service uses the events as delivered, so upstream filtering governs membership in the window.

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
