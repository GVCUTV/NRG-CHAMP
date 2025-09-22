# v3
# file: README.md
NRG-CHAMP Aggregator — Epoch-based Kafka reader/writer

This version implements the "epoca" mechanics:
- Reads every X milliseconds (epoch_ms) from assigned zone topics.
- Topics are per-zone; partitions are per-device (partition chosen by hash(deviceId)).
- For each epoch: round-robin over topics, and within each topic round-robin over partitions.
  As soon as a partition yields a message from the next epoch, we STOP and leave that message unread
  (offset not advanced) and move to the next partition.
- Overhead is stripped, outliers are discarded, and readings are grouped by device within the zone.
- After aggregating per zone+epoch, the aggregator writes the compact payload to:
  * MAPE topic (one topic; partition = hash(zoneId))
  * Ledger per-zone topic (two partitions: 0=aggregator, 1=mape). Aggregator writes to partition 0.
- All Kafka I/O is wrapped with the shared circuit breaker module.

Run locally:
  go run ./aggregator/cmd/server -props ./aggregator/aggregator.properties

Properties are in `aggregator.properties` (see that file for docs).