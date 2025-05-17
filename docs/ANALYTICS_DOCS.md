# Analytics Module Specifications

## Overview

The Analytics Module processes both real-time and historical data from the NRG CHAMP system—combining sensor readings and blockchain records—to compute key performance indicators (KPIs), generate energy-efficiency scores, and maintain leaderboards. It runs as a standalone microservice (module `it.uniroma2.dicii/nrg-champ/analytics`), deployable via Kubernetes.

- **Inputs**  
  - Time-series sensor data (via InfluxDB/Prometheus or message queue)  
  - Immutable transaction records (via Blockchain module API)  
- **Outputs**  
  - Computed KPIs and scores stored in a primary database  
  - Leaderboard snapshots and trend data  
  - REST API and WebSocket streams for front-end consumption  

## Functional Requirements

| ID        | Requirement                                                                                                                                     |
|-----------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| F5.1.1    | **Data Ingestion**<br/>Retrieve raw sensor metrics and blockchain transaction data at scheduled intervals (batch) or via streaming (real-time). |
| F5.1.2    | **KPI Computation**<br/>Calculate metrics such as `energyPerSqm`, `comfortIndex`, `anomalyCount`, and `uptimePercentage` for each zone.          |
| F5.1.3    | **Score Generation**<br/>Normalize KPIs into a unified `energyEfficiencyScore` (0–100 scale) per zone, applying configurable weighting rules.    |
| F5.1.4    | **Leaderboard Management**<br/>Maintain leaderboards for hierarchies (zone, floor, building, organization); update hourly and on-demand.         |
| F5.1.5    | **Trend Analysis**<br/>Provide time-series trend data (daily, weekly, monthly) for scores and KPIs.                                             |
| F5.1.6    | **On-Demand Queries**<br/>Expose REST endpoints to fetch KPIs, scores, leaderboards, and trends.                                                |
| F5.1.7    | **Real-Time Streaming**<br/>Optionally push updated scores and KPI events via WebSocket for subscribed front-end clients.                         |

## Non-Functional Requirements

| ID        | Requirement                                                                                                                   |
|-----------|-------------------------------------------------------------------------------------------------------------------------------|
| NF5.2.1   | **Performance**<br/>Batch computation of KPIs and leaderboards for 1,000 zones completes within 30 seconds.                  |
| NF5.2.2   | **Scalability**<br/>Able to scale horizontally to handle increased data volumes and concurrent API requests.                  |
| NF5.2.3   | **Reliability**<br/>Implement retry logic for data ingestion failures; ensure zero data loss during process interruptions.  |
| NF5.2.4   | **Security**<br/>Enforce TLS for all data transports; use JWT/RBAC for API access.                                          |
| NF5.2.5   | **Maintainability**<br/>Modular codebase with separate packages for ingestion, computation, storage, and API layers.        |
| NF5.2.6   | **Monitoring & Logging**<br/>Expose Prometheus metrics (processing time, error rates) and structured logs for auditing.      |
| NF5.2.7   | **Consistency**<br/>Ensure idempotent computations when replaying data; avoid duplicate score entries.                       |

## Data Model

```go
// pkg/models/analytics.go
package models

import "time"

// KPI stores computed key performance indicators for a zone.
type KPI struct {
    ZoneID          string    `json:"zoneId"`
    Timestamp       time.Time `json:"timestamp"`
    EnergyPerSqm    float64   `json:"energyPerSqm"`
    ComfortIndex    float64   `json:"comfortIndex"`
    AnomalyCount    int       `json:"anomalyCount"`
    UptimePercent   float64   `json:"uptimePercent"`
}

// Score represents normalized efficiency score for a zone.
type Score struct {
    ZoneID    string    `json:"zoneId"`
    Timestamp time.Time `json:"timestamp"`
    Value     float64   `json:"value"`    // 0–100
}

// LeaderboardEntry is one entry in a leaderboard.
type LeaderboardEntry struct {
    ZoneID string  `json:"zoneId"`
    Score  float64 `json:"score"`
    Rank   int     `json:"rank"`
}

// TrendPoint represents a time-series point for a zone.
type TrendPoint struct {
    Timestamp time.Time `json:"timestamp"`
    Value     float64   `json:"value"`
}
```

## API Specification

### REST Endpoints

```yaml
paths:
  /v1/analytics/kpis:
    get:
      summary: List KPIs
      security:
        - BearerAuth: []
      parameters:
        - name: start
          in: query
          schema: { type: string, format: date-time }
        - name: end
          in: query
          schema: { type: string, format: date-time }
        - name: zoneId
          in: query
          schema: { type: string }
        - name: page
          in: query
          schema: { type: integer, default: 1 }
        - name: pageSize
          in: query
          schema: { type: integer, default: 20 }
      responses:
        '200':
          description: Paginated list of KPIs
          content:
            application/json:
              schema:
                type: object
                properties:
                  total: { type: integer }
                  items:
                    type: array
                    items:
                      $ref: '#/components/schemas/KPI'

  /v1/analytics/scores:
    get:
      summary: List Scores
      security:
        - BearerAuth: []
      parameters:
        - name: zoneId
          in: query
          schema: { type: string }
        - name: page
          in: query
          schema: { type: integer, default: 1 }
        - name: pageSize
          in: query
          schema: { type: integer, default: 20 }
      responses:
        '200':
          description: Paginated list of Scores
          content:
            application/json:
              schema:
                type: object
                properties:
                  total: { type: integer }
                  items:
                    type: array
                    items:
                      $ref: '#/components/schemas/Score'

  /v1/analytics/leaderboards:
    get:
      summary: Get Leaderboard
      security:
        - BearerAuth: []
      parameters:
        - name: groupId
          in: query
          required: true
          schema: { type: string }
      responses:
        '200':
          description: Leaderboard for the group
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/LeaderboardEntry'

  /v1/analytics/trends:
    get:
      summary: Get Trend Data
      security:
        - BearerAuth: []
      parameters:
        - name: zoneId
          in: query
          schema: { type: string }
        - name: interval
          in: query
          schema: { type: string, enum: [daily, weekly, monthly] }
      responses:
        '200':
          description: Time-series trend data
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/TrendPoint'
```

### WebSocket

```
URL: wss://<host>/v1/analytics/stream
Subprotocol: Bearer <jwt-token>
Message format:
{
  "kpis": [ ...KPI ],
  "scores": [ ...Score ],
  "leaderboard": [ ...LeaderboardEntry ]
}
```

## Transaction Flow & Scheduling

1. **Batch Job**  
   - Runs hourly: fetch raw data → compute KPIs & scores → update DB & emit WebSocket.  
2. **On-Demand**  
   - Trigger via `POST /v1/analytics/compute?start=&end=` for manual recompute.  
3. **Streaming**  
   - Real-time events from message queue can feed incremental updates.

## Configuration & Deployment

| Key                         | Description                             | Default            |
|-----------------------------|-----------------------------------------|--------------------|
| `ANALYTICS_BATCH_CRON`      | Cron schedule for batch jobs            | `0 * * * *` (hourly) |
| `DB_CONN_STRING`            | Primary analytics database connection   | env var            |
| `PORT`                      | HTTP & WebSocket port                   | `8084`             |
| `LOG_LEVEL`                 | `debug`, `info`, `warn`, `error`        | `info`             |
| `MQ_KPI_TOPIC`              | Topic for incremental KPI events        | `analytics-events` |

## Future Enhancements

- Real-time stream processing (Kafka Streams/Flink)  
- Customizable weighting rules API  
- GraphQL endpoint for flexible queries  
- Integration with BI tools (Looker, Tableau)  

---
