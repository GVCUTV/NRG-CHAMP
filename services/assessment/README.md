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

## Observability
Metrics are exposed in Prometheus text format at `GET /metrics`. Standard counters, histograms, and gauges cover HTTP traffic, cache behaviour, and upstream Ledger availability. Examples:

```
# HELP http_requests_total Total count of HTTP requests processed by route and status.
# TYPE http_requests_total counter
http_requests_total{route="/kpi/summary",status="200"} 42

# HELP cache_hits_total Total cache hits observed.
# TYPE cache_hits_total counter
cache_hits_total 17

# HELP ledger_http_duration_seconds Histogram of Ledger HTTP request durations.
# TYPE ledger_http_duration_seconds histogram
ledger_http_duration_seconds_bucket{le="0.1"} 3
ledger_http_duration_seconds_bucket{le="0.25"} 7
ledger_http_duration_seconds_bucket{le="+Inf"} 9
ledger_http_duration_seconds_sum 1.82
ledger_http_duration_seconds_count 9

# HELP cb_state Circuit breaker state gauge (0 closed, 1 half, 2 open).
# TYPE cb_state gauge
cb_state{target="ledger"} 0
```
