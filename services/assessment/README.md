// v0
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
Optional `pages` parameter allows clients to declare the number of ledger pages a request would consume; values must not exceed `ASSESSMENT_MAX_PAGES`.

All requests are bounded by `ASSESSMENT_MAX_WINDOW`. When the caller omits `from`, the service trims the window to the smaller of one hour or the configured maximum.

## Environment
- `LEDGER_BASE_URL` (default `http://ledger:8084`)
- `TARGET_TEMP_C` (default `22`)
- `COMFORT_TOLERANCE_C` (default `0.5`)
- `CACHE_TTL` (default `30s`)
- `ASSESSMENT_BIND_ADDR` (default `:8085`)
- `ASSESSMENT_LOGFILE` (default `./assessment.log`)
- `ASSESSMENT_MAX_WINDOW` (default `24h`) — longest permissible `from`/`to` span per request.
- `ASSESSMENT_MAX_PAGES` (default `100`) — highest acceptable ledger page budget declared via the `pages` query parameter.

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
