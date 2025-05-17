# Blockchain Archiviation Specifications

## Overview

The Blockchain Archiviation module provides an immutable, verifiable ledger of all critical events in the NRG CHAMP system: environmental sensor batches and MAPE control actions. It runs as an independent microservice (module `it.uniroma2.dicii/nrg-champ/blockchain`), deployable and scalable via Kubernetes.

- **Inputs**  
  - Pre-processed sensor batches (published by the Aggregator service)  
  - Executed HVAC control commands (emitted by MAPE-Execute)  

- **Outputs**  
  - Transactions submitted to the configured blockchain network  
  - A local append-only ledger for retry/queueing  
  - REST API and WebSocket streams for external auditors, dashboards, and the front-end  

## Functional Requirements

| ID        | Requirement                                                                                                                                         |
|-----------|-----------------------------------------------------------------------------------------------------------------------------------------------------|
| F4.1.1    | **Data Aggregation**<br/>Collect sensor batches and control events (with timestamps, zone IDs, metadata). Validate completeness before packaging. |
| F4.1.2    | **Transaction Creation**<br/>Encapsulate each batch or event into a blockchain transaction, including: zoneId, timestamp, type (`sensor` or `command`), and payload. |
| F4.1.3    | **Transaction Submission**<br/>Submit transactions to the configured blockchain node or network endpoint; mark status `pending`, then `committed` or `failed`. |
| F4.1.4    | **Retry Mechanism**<br/>On submission failure (network down, node unreachable), enqueue locally and retry with exponential back-off until success or max attempts. |
| F4.1.5    | **Local Ledger Store**<br/>Maintain an append-only local ledger (LevelDB or JSON file) mirroring all attempted transactions and their statuses for audit and recovery. |
| F4.1.6    | **Query API**<br/>Expose RESTful endpoints (`GET /v1/ledger/transactions`, `/v1/ledger/transaction/{txId}`) supporting filters (date range, zoneId, type) and pagination. |
| F4.1.7    | **WebSocket Stream**<br/>Offer `wss://<host>/v1/blockchain/stream` delivering real-time notifications of `committed` transactions to subscribed clients. |
| F4.1.8    | **Access Control**<br/>Enforce JWT-based RBAC:<br/> - `auditor`: read-only access to REST and WebSocket streams<br/> - `admin`: full access, including internal metrics. |

## Non-Functional Requirements

| ID        | Requirement                                                                                                                           |
|-----------|-----------------------------------------------------------------------------------------------------------------------------------------|
| NF4.2.1   | **Performance**<br/>Capable of ≥100 transactions/sec submission rate under normal load.                                                 |
| NF4.2.2   | **Low Latency**<br/>Average time from data receipt to blockchain confirmation < 2 seconds.                                              |
| NF4.2.3   | **Scalability**<br/>Horizontally scalable: multiple replicas may run concurrently, each processing distinct partitions of input queues. |
| NF4.2.4   | **Reliability**<br/>99.9% service availability; automatic recovery via Kubernetes restarts.                                            |
| NF4.2.5   | **Fault Tolerance**<br/>Circuit-breaker pattern for blockchain client; local queue persists across restarts.                           |
| NF4.2.6   | **Security**<br/>TLS-encrypted communication to blockchain node; payloads signed or hashed per protocol.                                |
| NF4.2.7   | **Maintainability**<br/>Modular code; metrics/logs for Prometheus/Grafana.                                                             |
| NF4.2.8   | **Compliance & Auditability**<br/>All requests, retries, and state transitions logged; local ledger retains immutable history.         |

## Data Model

```go
// pkg/models/blockchain.go
package models

import "time"

// Transaction represents one blockchain submission.
type Transaction struct {
    TxID      string      `json:"txId"`
    ZoneID    string      `json:"zoneId"`
    Timestamp time.Time   `json:"timestamp"`
    Type      string      `json:"type"`    // "sensor" | "command"
    Payload   interface{} `json:"payload"`
    Status    string      `json:"status"`  // "pending" | "committed" | "failed"
}
```

## API Specification

### REST Endpoints

```yaml
paths:
  /v1/ledger/transactions:
    get:
      summary: List blockchain transactions
      security:
        - BearerAuth: []
      parameters:
        - $ref: '#/components/parameters/start'
        - $ref: '#/components/parameters/end'
        - $ref: '#/components/parameters/zoneId'
        - $ref: '#/components/parameters/type'
        - $ref: '#/components/parameters/page'
        - $ref: '#/components/parameters/pageSize'
      responses:
        '200':
          description: Paginated transactions
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/PaginatedTransactions'

  /v1/ledger/transactions/{txId}:
    get:
      summary: Get transaction by ID
      security:
        - BearerAuth: []
      parameters:
        - in: path
          name: txId
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Transaction details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Transaction'
        '404':
          description: Not found
```

### WebSocket

```
URL: wss://<host>/v1/blockchain/stream
Subprotocol: Bearer <jwt-token>
Message format:
{
  "event": "committed",  // "pending" | "failed"
  "transaction": { ...Transaction }
}
```

## Transaction Flow & Error Handling

1. **Receive Batch/Event**  
2. **Validate & Package**  
3. **Create Local Entry** (`pending`)  
4. **Submit to Blockchain**  
   - On success → mark `committed`, emit WebSocket  
   - On failure → keep `pending`, schedule retry  
5. **Retry Loop** (exponential back-off, up to N attempts)  
6. **Failure** → mark `failed`, alert via Prometheus

## Configuration & Deployment

| Key                         | Description                                      | Default             |
|-----------------------------|--------------------------------------------------|---------------------|
| `BC_NODE_URL`               | Blockchain node endpoint                         | `http://node:8545`  |
| `BC_RETRY_MAX_ATTEMPTS`     | Max submission retries                           | `10`                |
| `BC_RETRY_INITIAL_INTERVAL` | Back-off start (seconds)                         | `1`                 |
| `BC_RETRY_MAX_INTERVAL`     | Back-off max (seconds)                           | `60`                |
| `BC_LOCAL_LEDGER_PATH`      | Local ledger store path                          | `/data/ledger.db`   |
| `PORT`                      | HTTP & WebSocket port                            | `8083`              |
| `LOG_LEVEL`                 | `debug`, `info`, `warn`, `error`                 | `info`              |

## Extensibility & Future Work

- Smart Contracts for on-chain logic  
- Multi-network support (testnets, mainnet)  
- GraphQL or The Graph integration for advanced queries  
- Off-chain encryption + on-chain hashes for GDPR  

---
