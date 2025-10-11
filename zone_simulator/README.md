<!-- v3 -->
<!-- README.md -->
# Zone Simulator (NRG CHAMP) - final

This module simulates devices belonging to **a zone**. Features in this bundle:

- Per-partition Kafka consumption for actuators (each actuator reads the partition computed from its device UUID using CRC32 — this mirrors the producer's `kafka.Hash` behavior).
- Device IDs can be provided in `sim.properties` (`device.tempSensorId`, `device.heatId`, `device.coolId`, `device.fanId`). If absent, UUIDs are generated at startup.
- Per-device sampling rate overrides via `device.<uuid>.rate` keys.
- Actuator telemetry publishes instantaneous `powerKW` values for heating, cooling and ventilation devices.
- HTTP endpoints for health, status and manual actuator commands.

Kafka topic naming: `<TOPIC_PREFIX>.<zoneId>` (default `device.readings.<zoneId>`). Producer uses the device UUID as message key so messages for the same device consistently land on the same partition — consumers in this package open the specific partition corresponding to each actuator's deviceId

Consumer behavior:
- For each actuator (heating, cooling, ventilation) the simulator opens a **partition-scoped** reader and processes only messages whose `deviceId` equals the actuator id. It applies the most recent command received for that device.
- This minimizes the read overhead and isolates actuators to their partitions (Option B requested).

Deployment artifacts are included in the deploy package (Dockerfile, docker-compose.yml and Kubernetes manifests). See DEPLOY_README.md for instructions.

## Circuit Breaker Integration

- **Environment variables**: `CB_ENABLED`, `CB_KAFKA_FAILURE_THRESHOLD`, `CB_KAFKA_SUCCESS_THRESHOLD`, `CB_KAFKA_OPEN_SECONDS`, `CB_KAFKA_TIMEOUT_MS`, `CB_KAFKA_BACKOFF_MS` (defaults wired in Dockerfile/configmap).
- **Logging**: circuit breaker initialization emits `[CB] kafka: initialized with failureThreshold=<N>, openSeconds=<S>` and transitions log `[CB] kafka: state=OPEN after <failures> consecutive failures` / `[CB] kafka: state=CLOSED after <successes> consecutive successes`.
- **Exclusions**: none — all Kafka producers/consumers in the simulator are breaker-protected.
