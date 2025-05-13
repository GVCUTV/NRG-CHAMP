
# NRG CHAMP API Documentation

## General Configuration

### CORS Configuration
- **Allowed Origins:** `*` (for external front-end access)  
- **Allowed Methods:** `GET`, `POST`, `PUT`, `DELETE`, `OPTIONS`  
- **Allowed Headers:** `Authorization`, `Content-Type`

### Authentication & Authorization
- All endpoints require **JWT** tokens for authentication.
- **Roles:**  
  - `admin`: Full access (control, data access, configuration).  
  - `auditor`: Read-only access to blockchain and analytics.  
  - `occupant`: Read-only access to sensor data and performance scores.

## 1. **Aggregator Module**

### 1.1 Data Retrieval (Sensor Data)

#### **GET /sensors/{sensorId}/latest**  
- **Description:** Fetch the most recent reading from a specific sensor.
- **Permissions:**  
  - `occupant`: Read access for own sensors.  
  - `auditor`: Read access for all sensors.

##### **Response Data Contract:**
```json
{
  "sensorId": "sensor-01",
  "timestamp": "2025-05-11T12:00:00Z",
  "temperatureC": 22.5,
  "humidityPct": 45.0
}
```

---

#### **GET /sensors/{sensorId}/history?start=&end=&zoneId=&page=&pageSize=**  
- **Description:** Fetch historical sensor readings between the given time range.
- **Permissions:**  
  - `occupant`: Read access for own zone’s data.  
  - `auditor`: Read access for all data.

##### **Response Data Contract:**
```json
{
  "sensorId": "sensor-01",
  "readings": [
    {
      "timestamp": "2025-05-11T12:00:00Z",
      "temperatureC": 22.5,
      "humidityPct": 45.0
    },
    {
      "timestamp": "2025-05-11T12:05:00Z",
      "temperatureC": 22.4,
      "humidityPct": 44.9
    }
  ]
}
```

---

## 2. **MAPE Module (Monitor + Analyze)**

### 2.1 Dynamic Control

#### **POST /hvac/commands**  
- **Description:** Send control commands to the HVAC system (e.g., setpoint, fan speed).
- **Permissions:**  
  - `admin`: Full control over HVAC systems.  
  - `occupant`: Can only view the control status but cannot send commands.

##### **Request Data Contract:**
```json
{
  "zoneId": "zone-01",
  "action": "setPoint",
  "value": 22
}
```

---

#### **GET /hvac/status?zoneId={zoneId}**  
- **Description:** Fetch the current status of the HVAC system in the given zone.
- **Permissions:**  
  - `occupant`: Read access for own zone.  
  - `admin`: Read access for all zones.

##### **Response Data Contract:**
```json
{
  "zoneId": "zone-01",
  "currentSetPoint": 22,
  "currentTemp": 21.8,
  "fanSpeed": 2,
  "lastUpdate": "2025-05-11T12:00:30Z"
}
```
---

## 3. **Blockchain Module**

### 3.1 Blockchain Transactions

#### **GET /ledger/transactions**  
- **Description:** Fetch a list of blockchain transactions.
- **Permissions:**  
  - `auditor`: Read access to all transactions.  
  - `admin`: Full access to transaction metadata.

##### **Response Data Contract:**
```json
{
  "total": 100,
  "page": 1,
  "pageSize": 20,
  "transactions": [
    {
      "txId": "abc123",
      "zoneId": "zone-01",
      "timestamp": "2025-05-11T12:00:00Z",
      "type": "sensor",
      "payload": {"temperatureC": 22.5},
      "status": "committed"
    }
  ]
}
```

---

#### **GET /ledger/transactions/{txId}**  
- **Description:** Fetch details of a single transaction by its `txId`.
- **Permissions:**  
  - `auditor`: Read access for all transactions.
  - `admin`: Full access to transaction metadata.

##### **Response Data Contract:**
```json
{
  "txId": "abc123",
  "zoneId": "zone-01",
  "timestamp": "2025-05-11T12:00:00Z",
  "type": "sensor",
  "payload": {"temperatureC": 22.5},
  "status": "committed"
}
```
---

## 4. **Gamification Module**

### 4.1 Performance Scoring

#### **GET /leaderboards/{groupId}**  
- **Description:** Fetch the leaderboard for a specific group (e.g., floor, building).
- **Permissions:**  
  - `occupant`: Read access for their own zone and sub-group.
  - `admin`: Read access to all leaderboards.

##### **Response Data Contract:**
```json
{
  "groupId": "floor-01",
  "leaderboard": [
    {
      "zoneId": "zone-01",
      "score": 85,
      "anomalyCount": 2
    },
    {
      "zoneId": "zone-02",
      "score": 75,
      "anomalyCount": 5
    }
  ]
}
```

---

#### **GET /scores/{zoneId}**  
- **Description:** Fetch the performance score for a specific zone.
- **Permissions:**  
  - `occupant`: Read access for their own zone.
  - `admin`: Read access for all zones.

##### **Response Data Contract:**
```json
{
  "zoneId": "zone-01",
  "score": 85,
  "anomalyCount": 2,
  "trends": {
    "daily": 5,
    "weekly": 10
  }
}
```
---

## 5. **Assessment Module**

### 5.1 Data Metrics & Reports

#### **GET /metrics/energy-efficiency**  
- **Description:** Fetch the overall energy efficiency of a building.
- **Permissions:**  
  - `admin`: Full access to metrics and reports.  
  - `auditor`: Read-only access.

##### **Response Data Contract:**
```json
{
  "buildingId": "building-01",
  "energyEfficiency": 78.5,
  "savings": 1500
}
```

---

#### **GET /reports/compliance**  
- **Description:** Generate a compliance report for energy usage.
- **Permissions:**  
  - `admin`: Full access to compliance reports.  
  - `auditor`: Read-only access.

##### **Response Data Contract:**
```json
{
  "buildingId": "building-01",
  "complianceStatus": "Compliant",
  "auditDate": "2025-05-11",
  "detailedResults": { /* compliance details */ }
}
```
---

## Integration Points Summary

- **Data Contracts:**  
  - **Sensors:** `GET /sensors/{sensorId}/latest`, `GET /sensors/{sensorId}/history`
  - **HVAC Control:** `POST /hvac/commands`, `GET /hvac/status`
  - **Blockchain:** `GET /ledger/transactions`, `GET /ledger/transactions/{txId}`
  - **Gamification:** `GET /leaderboards/{groupId}`, `GET /scores/{zoneId}`
  - **Assessment:** `GET /metrics/energy-efficiency`, `GET /reports/compliance`

- **CORS:**  
  - **Allowed Origins:** `*` for front-end access  
  - **Allowed Methods:** `GET`, `POST`, `PUT`, `DELETE`, `OPTIONS`  
  - **Allowed Headers:** `Authorization`, `Content-Type`
