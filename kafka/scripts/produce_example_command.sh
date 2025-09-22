# v0
# scripts/produce_example_command.sh
#!/usr/bin/env bash
set -euo pipefail
TOPIC="${1:-device.readings.zone-A}"
DEVICE_ID="${2:-11111111-2222-3333-4444-555555555555}"
DEVICE_TYPE="${3:-act_heating}"   # act_heating | act_cooling | act_ventilation
STATE="${4:-ON}"                   # ON/OFF or 0/25/50/75/100 for ventilation
BROKER="${BROKER:-localhost:9092}"

# Build JSON payload (reading.state is set as requested)
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
PAYLOAD=$(jq -cn --arg did "$DEVICE_ID" --arg dt "$DEVICE_TYPE" --arg st "$STATE" --arg ts "$TS" '
{
  deviceId: $did,
  deviceType: $dt,
  zoneId: "zone-A",
  timestamp: $ts,
  reading: { state: $st }
}')

echo "Producing to $TOPIC with key=$DEVICE_ID"
# Use kcat to send key:value (key is critical for partition mapping)
docker run --rm --network host edenhill/kcat:1.7.1   -P -b "$BROKER" -t "$TOPIC" -K: -l <(printf "%s:%s
" "$DEVICE_ID" "$PAYLOAD")
