# NRG CHAMP Microservices API

## Common
- All services expose `GET /health` returning `{"status":"ok"}`.
- All APIs use JSON; error responses are `{ "error": "..." }`.

---

## Aggregator Service (port 8081)

### GET /batches/latest
Returns the most recent batch of cleaned sensor readings.
- **Response 200**  
  ```json
  [
    {
      "sensorId": "sensor-01",
      "timestamp": "2025-05-11T...Z",
      "temperatureC": 21.7,
      "humidityPct": 43.2
    }
  ]

* **Response 500** on server error

---

## MAPE-Execute Service (port 8082)

### POST /hvac/commands

Submit a control command to the external actuator system.

* **Body**

  ```json
  {
    "zoneId": "zone-01",
    "action": "setPoint",
    "value": 22
  }
  ```
* **202** – accepted
* **400** – invalid request

### GET /hvac/status?zoneId={zoneId}

Retrieve current HVAC status for a zone.

* **Response 200**

  ```json
  {
    "zoneId": "zone-01",
    "currentSetPoint": 22,
    "currentTemp": 21.9,
    "fanSpeed": 2,
    "lastUpdate": "2025-05-11T12:34:56Z"
  }
  ```
* **400** – missing zoneId

---

## Blockchain Service (port 8083)

### GET /ledger/transactions

List blockchain transactions with optional query params:

* `start` (ISO 8601), `end` (ISO 8601), `zoneId`, `page` (int), `pageSize` (int)
* **200**

  ```json
  {
    "total": 0,
    "page": 1,
    "pageSize": 20,
    "transactions": []
  }
  ```

### GET /ledger/transactions/{txId}

Get a single transaction.

* **Response 200**

  ```json
  {
    "txId": "abc123",
    "zoneId": "zone-01",
    "timestamp": "2025-05-11T...Z",
    "type": "sensor",
    "payload": { /* ... */ },
    "status": "committed"
  }
  ```
* **404** if not found