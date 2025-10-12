// v1
// README.md
# Assessment Service (Service 5)

**Purpose**: Compute KPIs strictly from the **Ledger** service and expose them via HTTP.

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

| Variable | Default | Description |
| --- | --- | --- |
| `LEDGER_BASE_URL` | `http://ledger:8084` | Ledger base URL for sourcing events. |
| `TARGET_TEMP_C` | `22` | Desired comfort temperature in Celsius. |
| `COMFORT_TOLERANCE_C` | `0.5` | Allowed deviation from the target temperature. |
| `CACHE_TTL` | `30s` | Legacy fallback applied when per-endpoint TTLs are unset. |
| `SUMMARY_CACHE_TTL` | inherits `CACHE_TTL` | TTL for summary endpoint cache entries. |
| `SERIES_CACHE_TTL` | inherits `CACHE_TTL` | TTL for series endpoint cache entries. |
| `ASSESSMENT_BIND_ADDR` | `:8085` | HTTP listen address. |
| `ASSESSMENT_LOGFILE` | `./assessment.log` | File sink for slog dual logging. |

### TTL configuration examples

```bash
export CACHE_TTL=1m           # default fallback
export SUMMARY_CACHE_TTL=15s  # short-lived summaries
export SERIES_CACHE_TTL=45s   # longer series cache
```

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
