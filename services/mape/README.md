// v1
// README.md
# NRG CHAMP — Full MAPE (Monitor, Analyze, Plan, Execute)

This service:
- **Monitors** Kafka readings from `<TOPIC_PREFIX>` (default `device.readings`).
- **Analyzes** each room against user-defined targets from `config/targets.properties`, with a deadband.
- **Plans** simple actions (heater/cooler ON/OFF, fan %) using proportional control.
- **Executes** actions:
  - `EXECUTE_MODE=http` → calls Room Simulator HTTP endpoints (`/actuators/heating|cooling|ventilation`).
  - `EXECUTE_MODE=kafka` → publishes to `COMMAND_TOPIC_PREFIX.<roomId>`.

## Placement in your repo
Place the `mape/full/` folder at the repo root, side-by-side with `./room_simulator` to mirror your layout.

```
mape/full/
├── cmd/server/main.go
├── internal/
│   ├── api/api.go
│   ├── analyze/analyze.go
│   ├── config/config.go
│   ├── execute/execute.go
│   ├── kafkabus/bus.go
│   ├── monitor/monitor.go
│   └── plan/plan.go
├── config/targets.properties
├── Dockerfile
├── docker-compose.addon.yml
└── k8s/mape/full/
    ├── configmap.yaml
    ├── deployment.yaml
    ├── kustomization.yaml
    └── service.yaml
    └── targets-configmap.yaml
```

## Environment
See `internal/config/config.go` for defaults. Typical overrides:
```
KAFKA_BROKERS=kafka:9092
TOPIC_PREFIX=device.readings
COMMAND_TOPIC_PREFIX=device.commands
CONSUMER_GROUP=mape-full
CONTROL_INTERVAL=5s
DEADBAND_C=0.4
KP=12
MAX_FAN_PCT=100
TARGETS_FILE=./config/targets.properties
TARGETS_RELOAD_EVERY=15s
DEFAULT_TARGET_C=22.0
EXECUTE_MODE=http   # or kafka
ROOM_HTTP_BASE=http://room-simulator:8080  # supports {room}
LOG_LEVEL=INFO
```

## Kafka message format (from Room Simulator)
```json
{
  "deviceId": "uuid",
  "deviceType": "temp_sensor | act_heating | act_cooling | act_ventilation",
  "zoneId": "zone-A",
  "timestamp": "2025-09-06T15:00:00Z",
  "reading": { "... device specific ..." }
}
```
- Temperature: `{ "tempC": 25.1 }`  
- Heating/Cooling: `{ "state": "ON"|"OFF", "powerW": 1500, "energyKWh": 0.023 }`  
- Ventilation: `{ "state": "0|25|50|75|100", "powerW": 100, "energyKWh": 0.005 }`

## HTTP API
- `GET /health`
- `GET /status?roomId=zone-A` → shows current measured state, target, and the *planned* command.

## Run locally
```bash
# From repo root
cd mape/full
go run ./cmd/server
# or build
go build -o bin/mape ./cmd/server && ./bin/mape
```

## Docker Compose
Merge `docker-compose.addon.yml` with your stack:
```bash
docker compose -f docker-compose.yml -f mape/full/docker-compose.addon.yml up --build
```

## Kubernetes
```bash
kubectl apply -k mape/full/k8s/mape/full/
kubectl get pods -l app=mape-full
kubectl logs deploy/mape-full
```

## Notes
- We used `github.com/segmentio/kafka-go` as Kafka client because Kafka is required by the spec.
- If you prefer commands via Kafka only, set `EXECUTE_MODE=kafka` and have your device/actuator adapters consume `device.commands.<roomId>`.
- Targets hot-reload: edit `config/targets.properties` (or the `mape-full-targets` ConfigMap) and the service will pick up changes within `TARGETS_RELOAD_EVERY`.
