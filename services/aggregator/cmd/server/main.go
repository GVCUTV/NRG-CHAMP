// Package main v1
// file: main.go
// Aggregator service: validates sensor readings, buffers last 15 minutes, exposes query APIs.
// Design choices:
// - Pure standard library only (HTTP, JSON, time, sync, slog).
// - Kafka ingestion is handled by a sidecar (kcat) that POSTs to /ingest to respect the "std lib only" constraint.
// - Republish step is optional per scope; we re-expose via API which downstream services can query.
//
// Endpoints:
//
//	GET	/health
//	GET /latest?zoneId=Z
//	GET /series?zoneId=Z&from=RFC3339&to=RFC3339
//	POST /ingest (internal; supports NDJSON lines or JSON array of readings)
//
// Logging:
//
//	Uses slog, logs every operation to stdout. Ready for redirection to files in container/orchestrator.
package main

import "NRG-CHAMP/aggregator/internal"

func main() {
	internal.Start()
}
