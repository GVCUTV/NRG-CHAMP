# v0
# scripts/create_topic.sh
#!/usr/bin/env bash
set -euo pipefail
TOPIC="${1:-device.readings.zone-A}"
PARTITIONS="${2:-6}"
REPLICATION="${3:-1}"
BROKER="${BROKER:-localhost:9092}"
echo "Creating topic '$TOPIC' partitions=$PARTITIONS rf=$REPLICATION on $BROKER"
docker run --rm --network host bitnami/kafka:3.7   kafka-topics.sh --create --topic "$TOPIC" --bootstrap-server "$BROKER"   --partitions "$PARTITIONS" --replication-factor "$REPLICATION" || true
docker run --rm --network host bitnami/kafka:3.7   kafka-topics.sh --describe --topic "$TOPIC" --bootstrap-server "$BROKER"
