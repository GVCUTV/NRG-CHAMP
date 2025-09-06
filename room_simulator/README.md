# Room Simulator (NRG CHAMP)

A single module that simulates a **room** with:
- Temperature sensor
- Heating actuator (ON/OFF) with energy meter
- Cooling actuator (ON/OFF) with energy meter
- Ventilation actuator (0/25/50/75/100%) with energy meter

Each device has its **own UUID** and **sampling rate**, and publishes to Kafka topic:
```
<TOPIC_PREFIX>.<zoneId>   # defaults to device.readings.<zoneId>
```
Each message contains `deviceType` and a device-specific `reading` payload.

## Build
```bash
go build ./cmd/room-simulator
```

## Run (Docker)
```bash
docker build -t nrgchamp/room-simulator:dev .
docker run --rm -p 8080:8080   -e KAFKA_BROKERS=localhost:9092   -e TOPIC_PREFIX=device.readings   -e SIM_PROPERTIES=/app/sim.properties   -v $(pwd)/sim.properties:/app/sim.properties   nrgchamp/room-simulator:dev
```

## HTTP API
- `GET /health` → 200 OK
- `GET /status` → current state, device IDs, and energy meters
- `POST /actuators/heating`  body: `{ "state": "ON" | "OFF" }`
- `POST /actuators/cooling`  body: `{ "state": "ON" | "OFF" }`
- `POST /actuators/ventilation` body: `{ "level": 0 | 25 | 50 | 75 | 100 }`

## Configuration
- `SIM_PROPERTIES` (required): path to `.properties` file. Must contain `zoneId` and `listen_addr`.
- `KAFKA_BROKERS` (csv, default `kafka:9092`)
- `TOPIC_PREFIX` (default `device.readings`)

### sim.properties example
```properties
zoneId=zone-A
listen_addr=:8080
alpha=0.02
beta=0.5
initial_t_in=26.0
initial_t_out=32.0
step=1s
sensor_rate=2s
heat_rate=5s
cool_rate=5s
fan_rate=5s
heat_power_w=1500
cool_power_w=1200
fan_power_w_25=50
fan_power_w_50=100
fan_power_w_75=150
fan_power_w_100=200
```

## Kafka message format
```json
{
  "deviceId": "uuid-...",
  "deviceType": "temp_sensor | act_heating | act_cooling | act_ventilation",
  "zoneId": "zone-A",
  "timestamp": "2025-09-06T15:00:00Z",
  "reading": { "... device specific ..." }
}
```
- Temperature: `{ "tempC": 25.1 }`
- Heating/Cooling: `{ "state": "ON"|"OFF", "powerW": 1500, "energyKWh": 0.023 }`
- Ventilation: `{ "state": "0|25|50|75|100", "powerW": 100, "energyKWh": 0.005 }`
