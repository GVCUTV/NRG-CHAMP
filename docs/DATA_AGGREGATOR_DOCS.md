# Data Aggregator Module Specifications

## Overview

The Data Aggregator module ingests raw environmental readings from the external Sensor System, applies validation and pre-processing (e.g., buffering, outlier removal, batching), and publishes cleaned data for downstream consumers (MAPE Engine, Blockchain Archiviation, Analytics). It runs as a standalone microservice (module `it.uniroma2.dicii/nrg-champ/aggregator`), deployable and scalable via Kubernetes.

- **Inputs**  
  - Real-time sensor data from the external Sensor System via HTTP API or message queue (e.g., MQTT, Kafka).  
- **Outputs**  
  - Cleaned, batched sensor readings published to a message broker topic `sensor-batches`.  
  - REST API for retrieving latest and historical batches.  
  - Optional WebSocket stream for real-time batch updates.

## Functional Requirements

| ID        | Requirement                                                                                                                                                       |
|-----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| F1.1.1    | **Data Ingestion**<br/>Continuously consume raw sensor readings from the external Sensor System endpoint or topic.                                                   |
| F1.1.2    | **Buffering & Retry**<br/>Buffer incoming readings in memory or local storage; retry ingestion from external system on transient failures.                           |
| F1.1.3    | **Validation**<br/>Validate each reading against schema and value bounds (e.g., temperature between –10°C and 50°C); reject or flag invalid data.                   |
| F1.1.4    | **Batching & Windowing**<br/>Group validated readings into fixed time windows (configurable, default 1 minute) and form `SensorBatch` objects for distribution.        |
| F1.1.5    | **Outlier Detection & Smoothing**<br/>Apply simple filters: remove extreme spikes/drops and optionally smooth via moving average.                                    |
| F1.1.6    | **Publishing**<br/>Publish each cleaned `SensorBatch` to the message broker topic `sensor-batches` with exactly-once semantics if supported.                         |
| F1.1.7    | **REST API**<br/>Expose endpoints to fetch the latest batch and historical batches with filters (`/batches/latest`, `/batches?start=&end=&zoneId=`).                  |
| F1.1.8    | **WebSocket Stream**<br/>Provide `wss://<host>/v1/aggregator/stream` for front-ends subscribing to real-time cleaned batches.                                         |
| F1.1.9    | **Health & Metrics**<br/>Expose `/health` and Prometheus metrics (`ingest_rate`, `error_rate`, `batch_latency`, `buffer_size`).                                        |

## Non-Functional Requirements

| ID        | Requirement                                                                                                                          |
|-----------|--------------------------------------------------------------------------------------------------------------------------------------|
| NF1.2.1   | **Performance**<br/>Ingest and process ≥5,000 readings/sec with end-to-end latency < 200 ms.                                         |
| NF1.2.2   | **Scalability**<br/>Horizontally scalable: multiple replicas can consume distinct partitions and coordinate via shared message broker. |
| NF1.2.3   | **Reliability**<br/>Ensure no data loss: buffer persisted across restarts and back-pressured gracefully on downstream unavailability. |
| NF1.2.4   | **Security**<br/>TLS for all inbound/outbound connections; API secured via JWT.                                                      |
| NF1.2.5   | **Maintainability**<br/>Modular code with separate packages: ingestion, validation, batching, publishing, API.                      |
| NF1.2.6   | **Monitoring & Logging**<br/>Prometheus metrics and structured logs (ingestion errors, validation failures).                          |

## Data Model

```go
// pkg/models/aggregator.go
package models

import "time"

// SensorReading represents one raw sensor measurement.
type SensorReading struct {
    SensorID  string    `json:"sensorId"`
    Timestamp time.Time `json:"timestamp"`
    TemperatureC float64 `json:"temperatureC"`
    HumidityPct  float64 `json:"humidityPct"`
}

// SensorBatch groups multiple readings in a time window.
type SensorBatch struct {
    BatchID    string           `json:"batchId"`
    ZoneID     string           `json:"zoneId"`
    StartTime  time.Time        `json:"startTime"`
    EndTime    time.Time        `json:"endTime"`
    Readings   []SensorReading  `json:"readings"`
}
```

## API Specification

### REST Endpoints

```yaml
paths:
  /v1/batches/latest:
    get:
      summary: Get the latest sensor batch
      security:
        - BearerAuth: []
      responses:
        '200':
          description: Latest SensorBatch
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SensorBatch'

  /v1/batches:
    get:
      summary: Get historical sensor batches
      security:
        - BearerAuth: []
      parameters:
        - in: query
          name: start
          schema: { type: string, format: date-time }
        - in: query
          name: end
          schema: { type: string, format: date-time }
        - in: query
          name: zoneId
          schema: { type: string }
        - in: query
          name: page
          schema: { type: integer, default: 1 }
        - in: query
          name: pageSize
          schema: { type: integer, default: 20 }
      responses:
        '200':
          description: Paginated SensorBatch list
          content:
            application/json:
              schema:
                type: object
                properties:
                  total: { type: integer }
                  items:
                    type: array
                    items:
                      $ref: '#/components/schemas/SensorBatch'
```

### WebSocket

```
URL: wss://<host>/v1/aggregator/stream
Subprotocol: Bearer <jwt-token>
Message format:
{
  "batch": { ...SensorBatch }
}
```

## Configuration & Deployment

| Key                     | Description                                         | Default      |
|-------------------------|-----------------------------------------------------|--------------|
| `AGG_INPUT_URL`         | URL or broker address for external sensor system    | `http://sensor-system:8000` |
| `AGG_BATCH_WINDOW_SEC`  | Time window for batching (seconds)                  | `60`         |
| `AGG_MAX_BUFFER_SIZE`   | Max in-memory buffered readings                     | `10000`      |
| `PORT`                  | HTTP & WebSocket port                               | `8081`       |
| `LOG_LEVEL`             | `debug`, `info`, `warn`, `error`                    | `info`       |
| `BROKER_ADDR`           | Message broker address (Kafka/MQTT)                 | `kafka:9092` |
| `OUTPUT_TOPIC`          | Topic for cleaned batches                           | `sensor-batches` |

## Processing Flow

1. **Consume** raw readings from external Sensor System.  
2. **Validate** and buffer; retry on failures.  
3. **Batch** readings every window; apply filtering/smoothing.  
4. **Publish** `SensorBatch` to broker.  
5. **Serve** via REST and WebSocket for front-end.  

---

*End of Data Aggregator Module Specifications*
