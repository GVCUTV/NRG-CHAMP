# Project Overview
- **Name:** NRG CHAMP (Responsive Goal-driven Cooling and Heating Automated Monitoring Platform)  
- **Goal:** Optimize HVAC energy consumption in large corporate/public buildings via IoT, MAPE control loop, blockchain archiving, and gamification.  
- **Tech Stack:**  
  - Language: Go (≥1.23.4)  
  - Communication: MQTT/TLS for sensors, HTTP/REST for APIs, Kafka/RabbitMQ for internal messaging  
  - Data Stores: InfluxDB/Prometheus (time-series), LevelDB or JSON file (local ledger), relational/NoSQL (analytics), blockchain network  
  - Containerization: Docker multi-stage builds, runtime on Alpine  
  - Orchestration: Kubernetes (deployments, services, healthChecks)  
  - CI/CD: GitHub Actions pipelines  
  - Monitoring: Prometheus/Grafana  
  - Security: JWT Authentication, RBAC, TLS encryption  

# Core Modules
1. **Device Module**  
   - Subcomponents: Device logic, Actuator simulators, Sensor simulators  
2. **Data Aggregator**  
3. **MAPE Module**  
   - Submodules: Monitor, Analyze, Plan, Execute  
4. **Blockchain Archiving Module**  
5. **Gamification Module**  
6. **Assessment Module**  

# Functional & Non-Functional Requirements
## Functional Requirements  
- **Sensor Data Ingestion:** Validate, buffer, aggregate IoT readings (temp, hum, CO₂).  
- **MAPE Control:**  
  - Monitor real-time data  
  - Analyze for inefficiencies/anomalies  
  - Plan optimized setpoints  
  - Execute HVAC commands with ACK feedback  
- **Blockchain Archiving:**  
  - Package sensor+command data into transactions  
  - Submit to blockchain node; retry on failure  
  - Expose REST API for external auditors (filter by time/zone)  
- **Analytics & Gamification:**  
  - Compute KPIs (kWh/m², comfort index)  
  - Generate normalized scores  
  - Maintain public/private leaderboards  
- **APIs:** CRUD for sensors, HVAC control, ledger queries, gamification data; secure via JWT/RBAC  
- **Dashboard:** Real-time charts, control panel, ledger viewer, leaderboards, alerts  

## Non-Functional Requirements  
- **Performance:** <500 ms control-loop latency; ≥100 TPS blockchain throughput  
- **Scalability:** Horizontal scaling of microservices; pluggable blockchain backends  
- **Security:** TLS everywhere; encrypted at rest; RBAC for APIs  
- **Reliability:** 99.9% uptime; retry logic, circuit breakers  
- **Maintainability:** Modular code, Documentation, Logging, Metrics  
- **Compliance:** Audit ready for energy & privacy regulations  

# Detailed Use Cases
| UC-ID | Name                                       | Actors                                             |
|-------|--------------------------------------------|----------------------------------------------------|
| UC-1  | Data Collection & Environmental Monitoring | IoT Sensors, Data Aggregator, BMS                  |
| UC-2  | Dynamic HVAC Control via MAPE Cycle        | MAPE Engine modules, HVAC Systems                  |
| UC-3  | Blockchain Data Recording & Transparency   | Data Recorder, Blockchain Network, External Auditors |
| UC-4  | Data Analysis & Gamification               | Analytics Engine, Gamification Module, End Users   |
| UC-5  | External Transparency & Regulatory Query   | External Auditors, API Gateway, Ledger Query Service |
| UC-6  | System Scalability & Adaptability          | System Admins, Config Service, Microservices       |

Each use case includes:
- **Preconditions** (e.g., sensors installed, network available)  
- **Basic Flow** steps 1…n  
- **Alternative Flows** (network down, invalid data, timeout)  
- **Extension Points** (manual override, external weather integration)  
- **Inclusions** (error logging, audit trail)  

# System Architecture Sketches
- **Component Diagram:**  
  - Sensors → Ingestion → Aggregator → { MAPE Engine → HVAC Systems ; Blockchain Module → Blockchain Network } → Analytics → APIs  
- **Sequence Diagram:**  
  ```
  DataAggregator -> TransactionBuilder: sendBatch(data)
  TransactionBuilder -> BlockchainModule: buildTransaction()
  BlockchainModule -> BlockchainNode: submit(tx)
  BlockchainNode --> BlockchainModule: ack/receipt(txId)
  BlockchainModule -> LocalLedger: store & update status
  ```
- **API Gateway:** JWT/RBAC, routes to sensor, control, ledger, gamification services  

# MVP Scope & Module Breakdown
## 6.1. Sensor Simulation & Data Aggregation
- Virtual sensors per zone with configurable sampling/noise/failures  
- Ingestion: MQTT/HTTP, JSON schema, buffer & retry  
- Aggregator: 1-minute windowing, smoothing, fan-out to TSDB & queue  

## 6.2. MAPE Engine
- **Monitor:** Subscribe to queue, maintain sliding window  
- **Analyze:** Rule thresholds, anomaly detection  
- **Plan:** Rule-based setpoint changes, multi-zone coordination  
- **Execute:** REST/MQTT commands, ACK handling, feedback loop  

## 6.3. Blockchain Simulation
- **Transaction Builder:** Serialize sensor summaries + command logs + metadata  
- **Local Ledger:** Append-only JSON/LevelDB store, optional block grouping  
- **Retry & Cache:** Exponential backoff, dead-letter queue  
- **Query API:** GET `/ledger/transactions`, `/ledger/transaction/{txId}` with pagination/filter  

## 6.4. Analytics & Gamification
- Batch jobs (hourly) compute KPIs; future real-time streams  
- Scoring: energy efficiency, anomaly penalties, normalization  
- Leaderboards: public & private, trend sparklines  

## 6.5. API Endpoints
| Path                                       | Method | Auth Role | Description                              |
|--------------------------------------------|--------|-----------|------------------------------------------|
| `/api/v1/sensors/{id}/latest`              | GET    | occupant  | Latest reading                           |
| `/api/v1/sensors/{id}/history`             | GET    | occupant  | Historical data slice                    |
| `/api/v1/hvac/commands`                    | POST   | occupant  | Submit manual override                   |
| `/api/v1/hvac/status`                      | GET    | occupant  | Current setpoints & ACKs                 |
| `/api/v1/ledger/transactions`              | GET    | auditor   | List transactions (filters + pagination) |
| `/api/v1/ledger/transaction/{txId}`        | GET    | auditor   | Single transaction fetch                 |
| `/api/v1/leaderboards/{groupId}`           | GET    | occupant  | Fetch leaderboard                        |
| `/api/v1/scores/{zoneId}`                  | GET    | occupant  | Fetch zone score                         |

## 6.6. Initial Dashboard Requirements
- **Real-Time Sensor Chart:** WebSocket feed, zone selector  
- **HVAC Control Panel:** Current setpoint, ±1 °C buttons, fan speed controls  
- **Blockchain Audit Viewer:** Table with TxID, zoneId, type, timestamp, status  
- **Gamification Leaderboard:** Rank, zone/floor, score, anomalies  
- **Alerts & Notifications:** Sensor offline, command failures, ledger backlog  

# Blockchain Integration
- **Component Flow:**  
  1. Data Aggregator → Transaction Builder (queue)  
  2. Transaction Builder → Blockchain Module  
  3. Blockchain Module ↔ Blockchain Network (submit & ack)  
  4. API Gateway ↔ Ledger Query API ↔ External Auditors  
- **Sequence & Error Flows:**  
  - Network failure → retry/backoff  
  - Data validation failure → reject/log  
  - ACK timeout → mark pending/retry threshold  
- **OpenAPI Snippet:**  
  ```yaml
  openapi: 3.0.3
  info:
    title: NRG CHAMP Ledger API
    version: 1.0.0
  paths:
    /v1/ledger/transactions:
      get:
        security: [ BearerAuth: [] ]
        parameters:
          - name: start; in: query; schema: {type: string, format: date-time}
          - name: end; in: query;   schema: {type: string, format: date-time}
          - name: zoneId; in: query; schema: {type: string}
          - name: page; in: query;   schema: {type: integer, example: 1}
          - name: pageSize; in: query; schema: {type: integer, example: 20}
        responses:
          '200': { description: OK, content: {application/json: {schema: {type: object}}}}
    /v1/ledger/transaction/{txId}:
      get:
        security: [ BearerAuth: [] ]
        parameters:
          - name: txId; in: path; required: true; schema: {type: string}
        responses:
          '200': { description: Transaction details }
          '404': { description: Not found }
  components:
    securitySchemes:
      BearerAuth:
        type: http
        scheme: bearer
        bearerFormat: JWT
    schemas:
      Transaction:
        type: object
        properties:
          txId:      { type: string }
          zoneId:    { type: string }
          timestamp: { type: string, format: date-time }
          type:      { type: string, enum: [sensor,command] }
          payload:   { type: object }
          status:    { type: string, enum: [pending,committed,failed] }
  ```
  
# Task Schedule & Day-by-Day Progress
| Day   | Giuseppe Tasks                                                           | Andrea Tasks                                                      | Shared Outcome                                    |
|-------|---------------------------------------------------------------------------|-------------------------------------------------------------------|---------------------------------------------------|
| Day 1 | Kickoff meeting; refine requirements; outline blockchain/ciphering scope  | Document MVP scope; draft architecture sketches                  | Aligned on MVP scope & simulation components      |
| Day 2 | Break down MVP modules; generate design diagrams & API specs              | Create task board; Docker-Compose setup; front-end scaffolding   | Synced tasks; defined integration & CI/CD ideas   |
| Day 3 | Configure env & Copilot; scaffold Go boilerplate for blockchain modules   | Set up Kubernetes; stub sensor simulator; front-end init         | Reviewed env setup; ensured repo consistency      |

# Environment & Tool Setup
- **Go Installation:**  
  ```bash
  go version           # -> go1.23.4 darwin/amd64
  go env GOPATH
  which go
  ```
- **Shell Config:**  
  ```bash
  export GOPATH="$HOME/go"
  export PATH="$PATH:/usr/local/go/bin:$GOPATH/bin"
  ```
- **GitHub Copilot:**  
  - Install VS Code extension; log in with GPT Plus  
- **Dependencies Initialization:**  
  ```bash
  go mod init github.com/your-org/nrg-champ-ledger
  go get github.com/gorilla/mux github.com/google/uuid github.com/spf13/viper
  ```

# Docker & Kubernetes Configuration
## Folder Structure
```
/device/
/data-aggregator/
/mape/monitor/
/mape/analyze/
/mape/plan/
/mape/execute/
/ledger/
/gamification/
/assessment/
/deploy/k8s/
```

## Sample Multi-Stage Dockerfile (Device Module)
```dockerfile
# Stage 1: Build
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o device-app ./cmd

# Stage 2: Runtime
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/device-app .
ENV LOG_LEVEL=info LOG_FORMAT=json
EXPOSE 8081
HEALTHCHECK --interval=30s --timeout=5s   CMD wget --no-verbose --tries=1 http://localhost:8081/health || exit 1
ENTRYPOINT ["./device-app"]
```

## Kubernetes Manifests (deploy/k8s/device-deployment.yaml)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: device-deployment
spec:
  replicas: 2
  selector:
    matchLabels:
      app: device
  template:
    metadata:
      labels:
        app: device
    spec:
      containers:
        - name: device
          image: your-registry/nrg-champ-device:latest
          ports:
            - containerPort: 8081
          env:
            - name: LOG_LEVEL
              value: "info"
          readinessProbe:
            httpGet:
              path: /health
              port: 8081
            initialDelaySeconds: 5
            periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: device-service
spec:
  selector:
    app: device
  ports:
    - protocol: TCP
      port: 8081
      targetPort: 8081
```

