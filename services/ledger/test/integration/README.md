// v0
// services/ledger/test/integration/README.md
# Ledger Public Publish Integration Test

This directory hosts integration-level verification for the ledger service
ensuring finalized epochs produce the expected public payload.

## Running the test

Execute the test with the `integration` build tag:

```bash
go test -tags=integration ./services/ledger/test/integration -count=1
```

The test spins an in-memory publisher/consumer pair, so no external Kafka
cluster is required.
