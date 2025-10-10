<!-- v1 -->
<!-- README.md -->
# Zone Simulator (NRG CHAMP) - final

This module simulates devices belonging to **a zone**. Features in this bundle:

- Per-partition Kafka consumption for actuators (each actuator reads the partition computed from its device UUID using CRC32 — this mirrors the producer's `kafka.Hash` behavior).
- Device IDs can be provided in `sim.properties` (`device.tempSensorId`, `device.heatId`, `device.coolId`, `device.fanId`). If absent, UUIDs are generated at startup.
- Per-device sampling rate overrides via `device.<uuid>.rate` keys.
- HTTP endpoints for health, status and manual actuator commands.

Kafka topic naming: `<TOPIC_PREFIX>.<zoneId>` (default `device.readings.<zoneId>`). Producer uses the device UUID as message key so messages for the same device consistently land on the same partition — consumers in this package open the specific partition corresponding to each actuator's deviceId

Consumer behavior:
- For each actuator (heating, cooling, ventilation) the simulator opens a **partition-scoped** reader and processes only messages whose `deviceId` equals the actuator id. It applies the most recent command received for that device.
- This minimizes the read overhead and isolates actuators to their partitions (Option B requested).

Deployment artifacts are included in the deploy package (Dockerfile, docker-compose.yml and Kubernetes manifests). See DEPLOY_README.md for instructions.
