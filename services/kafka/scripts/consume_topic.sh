# v0
# scripts/consume_topic.sh
#!/usr/bin/env bash
set -euo pipefail
TOPIC="${1:-device.readings.zone-A}"
BROKER="${BROKER:-localhost:9092}"
echo "Consuming from $TOPIC on $BROKER"
docker run --rm --network host edenhill/kcat:1.7.1   -C -b "$BROKER" -t "$TOPIC" -o end -q
