# MAPE Engine Module Specifications

The MAPE (Monitor, Analyze, Plan, Execute) Engine dynamically controls HVAC operations based on real-time sensor data. Each sub-module runs as its own microservice under `it.uniroma2.dicii/nrg-champ/mape-<submodule>`.

---

## 1. Monitor Sub-Module

### Overview
Consumes cleaned sensor batches, maintains sliding-window state per zone, and publishes metrics for downstream analysis.

### Functional Requirements

| ID       | Requirement                                                                                                         |
|----------|---------------------------------------------------------------------------------------------------------------------|
| F2.1.1   | **Subscription**<br/>Consume sensor batches from message topic `sensor-batches`.                                    |
| F2.1.2   | **State Management**<br/>Maintain sliding window of the last N readings per zone (configurable window size).       |
| F2.1.3   | **Metrics Emission**<br/>Expose Prometheus metrics: average temperature, humidity, and consumption per zone.        |
| F2.1.4   | **Anomaly Alerts**<br/>Detect missing batches or sensor inactivity and emit alert events.                          |
| F2.1.5   | **REST API**<br/>GET `/monitor/status?zoneId={zoneId}` returns current window summary (avg, min, max).             |
| F2.1.6   | **WebSocket**<br/>Stream real-time window summaries via `wss://<host>/v1/monitor/stream`.                          |

### Non-Functional Requirements

| ID         | Requirement                                                                                       |
|------------|---------------------------------------------------------------------------------------------------|
| NF2.1.1    | **Low Latency**<br/>Window summary update and metric emission within 100ms of batch arrival.      |
| NF2.1.2    | **Scalability**<br/>Handle 5,000 zones with sliding windows concurrently.                         |
| NF2.1.3    | **Reliability**<br/>Persist window state on restart; recover from last checkpoint.                  |

### Data Model

```go
// pkg/models/monitor.go
package models

import "time"

type WindowSummary struct {
    ZoneID      string    `json:"zoneId"`
    Timestamp   time.Time `json:"timestamp"`
    AvgTemp     float64   `json:"avgTemp"`
    MinTemp     float64   `json:"minTemp"`
    MaxTemp     float64   `json:"maxTemp"`
    AvgHumidity float64   `json:"avgHumidity"`
}
```

### API Specification

```yaml
paths:
  /v1/monitor/status:
    get:
      summary: Get current window summary for a zone
      parameters:
        - name: zoneId
          in: query
          required: true
          schema: { type: string }
      responses:
        '200':
          description: WindowSummary
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/WindowSummary'
```

---

## 2. Analyze Sub-Module

### Overview
Processes window summaries and historical data to detect anomalies and inefficiencies, generating insights.

### Functional Requirements

| ID         | Requirement                                                                                                  |
|------------|--------------------------------------------------------------------------------------------------------------|
| F2.2.1     | **Input Consumption**<br/>Consume `WindowSummary` from Monitor or historical data via REST API.               |
| F2.2.2     | **Anomaly Detection**<br/>Flag readings deviating >X% from baseline; generate `AnomalyEvent`.                |
| F2.2.3     | **Baseline Calculation**<br/>Retrieve historical same-hour/week data for comparison.                         |
| F2.2.4     | **Insight Generation**<br/>Produce suggestions (e.g., adjust setpoints by Y°C).                              |
| F2.2.5     | **REST API**<br/>GET `/analyze/anomalies?zoneId={zoneId}` and `/analyze/insights?zoneId={zoneId}`.           |

### Non-Functional Requirements

| ID         | Requirement                                                                    |
|------------|--------------------------------------------------------------------------------|
| NF2.2.1    | **Throughput**<br/>Process 10k window summaries per second.                    |
| NF2.2.2    | **Accuracy**<br/>Baseline lookup latency < 200ms.                              |

### Data Model

```go
// pkg/models/analyze.go
package models

import "time"

type AnomalyEvent struct {
    ZoneID    string    `json:"zoneId"`
    Timestamp time.Time `json:"timestamp"`
    Metric    string    `json:"metric"`   // "temperature"|"humidity"
    Value     float64   `json:"value"`
    Deviation float64   `json:"deviation"` // % deviation
}

type Insight struct {
    ZoneID    string `json:"zoneId"`
    Recommendation string `json:"recommendation"`
}
```

### API Specification

```yaml
paths:
  /v1/analyze/anomalies:
    get:
      summary: List anomalies for a zone
      parameters:
        - name: zoneId; in: query; required: true; schema: { type: string }
      responses:
        '200': { content: { application/json: { schema: { type: array, items: { $ref: '#/components/schemas/AnomalyEvent' } } } } }

  /v1/analyze/insights:
    get:
      summary: Get insights for a zone
      parameters:
        - name: zoneId;
          in: query;
          required: true;
          schema: {
            type: string
          }
      responses:
        '200': {
          content: {
            application/json: {
              schema: {
                type: array,
                items: {
                  $ref: '#/components/schemas/Insight'
                }
              }
            }
          }
        }
```

---

## 3. Plan Sub-Module

### Overview
Generates optimized HVAC control strategies based on insights, applying rule sets and multi-zone coordination.

### Functional Requirements

| ID         | Requirement                                                                                      |
|------------|--------------------------------------------------------------------------------------------------|
| F2.3.1     | **Input Consumption**<br/>Consume `Insight` events from Analyze via message queue or API webhook. |
| F2.3.2     | **Rule Engine**<br/>Apply configurable rules (e.g., if temp>25°C then setPoint=22°C).             |
| F2.3.3     | **Multi-Zone Coordination**<br/>Balance load across zones to optimize group energy usage.         |
| F2.3.4     | **Plan Packaging**<br/>Create `PlanPacket` objects (zoneId, setPoint, fanSpeed).                 |
| F2.3.5     | **REST API**<br/>GET `/plan/packet?zoneId={zoneId}`; POST `/plan/execute` to submit plans.       |

### Non-Functional Requirements

| ID         | Requirement                                                                |
|------------|----------------------------------------------------------------------------|
| NF2.3.1    | **Rule Latency**<br/>Compute plan within 50ms per zone.                   |
| NF2.3.2    | **Configurability**<br/>Support dynamic rule updates without restart.      |

### Data Model

```go
// pkg/models/plan.go
package models

import "time"

type PlanPacket struct {
    ZoneID    string    `json:"zoneId"`
    SetPoint  float64   `json:"setPoint"`
    FanSpeed  int       `json:"fanSpeed"`
    CreatedAt time.Time `json:"createdAt"`
}
```

### API Specification

```yaml
paths:
  /v1/plan/packet:
    get:
      summary: Get next plan packet for a zone
      parameters:
        - name: zoneId;
          in: query;
          required: true;
          schema: {
            type: string
          }
      responses:
        '200': {
          content: {
            application/json: {
              schema: {
                $ref: '#/components/schemas/PlanPacket'
              }
            }
          }
        }

  /v1/plan/execute:
    post:
      summary: Submit plan packets to Execute module
      requestBody:
        content:
          application/json:
            schema: {
              type: array,
              items: {
                $ref: '#/components/schemas/PlanPacket'
              }
            }
      responses:
        '202': {
          description: Accepted
        }
```

---

## 4. Execute Sub-Module

### Overview
Sends control commands to the external Actuator System, handles acknowledgements, and reports execution status.

### Functional Requirements

| ID         | Requirement                                                                                          |
|------------|------------------------------------------------------------------------------------------------------|
| F2.4.1     | **Input Consumption**<br/>Consume `PlanPacket` via REST or message queue.                             |
| F2.4.2     | **Command Dispatch**<br/>Map `PlanPacket` to actuator commands and send via HTTP or MQTT.             |
| F2.4.3     | **ACK Handling**<br/>Wait for ACK/NACK; retry up to N times; log failures.                            |
| F2.4.4     | **REST API**<br/>POST `/execute/commands`; GET `/execute/status?zoneId={zoneId}`.                     |
| F2.4.5     | **WebSocket**<br/>Stream execution status updates via `wss://<host>/v1/execute/stream`.               |

### Non-Functional Requirements

| ID         | Requirement                                                             |
|------------|---------------------------------------------------------------------------|
| NF2.4.1    | **Reliability**<br/>Ensure at-least-once delivery of commands.           |
| NF2.4.2    | **Latency**<br/>Complete one command round-trip within 500ms.            |

### Data Model

```go
// pkg/models/execute.go
package models

import "time"

type ExecuteCommand struct {
    ZoneID   string `json:"zoneId"`
    Action   string `json:"action"`   // "setPoint"|"fanSpeed"
    Value    int    `json:"value"`
}

type ExecuteStatus struct {
    ZoneID    string    `json:"zoneId"`
    Status    string    `json:"status"`    // "pending"|"success"|"failed"
    Timestamp time.Time `json:"timestamp"`
}
```

### API Specification

```yaml
paths:
  /v1/execute/commands:
    post:
      summary: Submit execution commands
      requestBody:
        content:
          application/json:
            schema: {
              type: array,
              items: {
                $ref: '#/components/schemas/ExecuteCommand'
                }
              }
      responses:
        '202': {
          description: Accepted
        }

  /v1/execute/status:
    get:
      summary: Get execution status for a zone
      parameters:
        - name: zoneId;
          in: query;
          required: true;
          schema: {
            type: string
          }
      responses:
        '200': {
          content: {
            application/json: {
              schema: {
                type: array,
                items: {
                  $ref: '#/components/schemas/ExecuteStatus'
                }
              }
            }
          }
        }
```

---
