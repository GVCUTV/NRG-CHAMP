# v0
# DEPLOY_KAFKA_README.md
# Kafka Cluster (KRaft) — Deployment Bundle

This bundle provides a **single‑broker Kafka (KRaft mode)** suitable for demos/labs where Kafka sits between the *Zone Simulator* and the *Aggregator*.

Contents
- `docker-compose.yml` — local, single-broker KRaft with Bitnami image (no ZooKeeper).
- `scripts/create_topic.sh` — create a topic with custom partitions/replication.
- `scripts/produce_example_command.sh` — send a MAPE-like command keyed by deviceId.
- `scripts/consume_topic.sh` — tail a topic.
- `k8s/kafka-deployment.yaml` — Kubernetes Deployment (single broker, KRaft).
- `k8s/kafka-service.yaml` — ClusterIP service (`kafka:9092` inside cluster).
- `k8s/create-topics-job.yaml` — one-off Job to create zone topics (optional).

## Why KRaft?
KRaft (Kafka Raft) removes ZooKeeper, making single-node demos simpler and lighter.

## Quick start — Docker Compose (local)
1) Launch Kafka:
```bash
docker compose up -d
```

2) Create a topic for a zone (e.g., `device.readings.zone-A`) with at least as many partitions as actuators in the zone:
```bash
./scripts/create_topic.sh device.readings.zone-A 6
```

3) Test produce a MAPE command for a heater device (replace UUID accordingly):
```bash
./scripts/produce_example_command.sh device.readings.zone-A 11111111-2222-3333-4444-555555555555 act_heating ON
```

4) Consume to verify:
```bash
./scripts/consume_topic.sh device.readings.zone-A
```

## Kubernetes (single-broker demo)
> Requires a cluster (kind/minikube/k3s) and `kubectl` configured.

1) Deploy Kafka and Service:
```bash
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
```

2) (Optional) Create topics with a Job:
Edit `k8s/create-topics-job.yaml` to set `TOPIC_NAME` and `PARTITIONS`, then:
```bash
kubectl apply -f k8s/create-topics-job.yaml
kubectl logs job/create-kafka-topics
```

3) Point your apps at:
- Inside cluster: `kafka:9092`
- Outside cluster (for port-forward): `kubectl port-forward svc/kafka 9092:9092` then `localhost:9092`

## Tips
- **Partitions**: For Option B (per‑partition consumer), ensure topic partitions ≥ number of devices that must be isolated by key.
- **Keys**: Producers *must* set the Kafka message key to the device UUID so hashing sends all messages for that device to the same partition.
- **Replication**: This is a single broker demo → replication factor 1, not fault tolerant.
