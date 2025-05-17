# Gamification Module Specifications

## Overview

The Gamification Module incentivizes energy-efficient behaviors by computing, storing, and serving scores, leaderboards, and reward mechanisms. It ingests KPI and score data from the Analytics Module and exposes REST and WebSocket interfaces for front-end consumption. It runs as an independent microservice (module `it.uniroma2.dicii/nrg-champ/gamification`), deployable via Kubernetes.

- **Inputs**  
  - Normalized `Score` objects from Analytics Module  
  - Optional manual inputs (e.g., bonus points, challenge definitions)  
- **Outputs**  
  - Leaderboards for hierarchies (zone, floor, building)  
  - Individual score details  
  - Reward events (optional integrations)  
  - REST API and WebSocket streams for front-end  

## Functional Requirements

| ID         | Requirement                                                                                                                                      |
|------------|--------------------------------------------------------------------------------------------------------------------------------------------------|
| F6.1.1     | **Score Ingestion**<br/>Consume `Score` updates from the Analytics Module via REST or message queue for real-time processing.                    |
| F6.1.2     | **Leaderboard Computation**<br/>Aggregate and sort scores by group (zone, floor, building, organization) to produce leaderboards.               |
| F6.1.3     | **Custom Challenges**<br/>Allow administrators to define time-bound challenges with custom scoring rules and groups.                             |
| F6.1.4     | **Reward Management**<br/>Trigger reward events (e.g., token issuance, notifications) based on score thresholds or challenge outcomes.           |
| F6.1.5     | **REST API**<br/>Expose endpoints to fetch leaderboards, individual scores, challenge definitions, and reward histories.                         |
| F6.1.6     | **WebSocket Stream**<br/>Provide `wss://<host>/v1/gamification/stream` for real-time leaderboard and score updates to subscribed clients.         |
| F6.1.7     | **Access Control**<br/>JWT-based RBAC:<br/> - `occupant`: read access to own zone scores and leaderboards<br/> - `admin`: full access including challenge and reward APIs |

## Non-Functional Requirements

| ID         | Requirement                                                                                                                                              |
|------------|----------------------------------------------------------------------------------------------------------------------------------------------------------|
| NF6.2.1    | **Performance**<br/>Compute and serve leaderboards for up to 5,000 zones within 500 ms under normal load.                                               |
| NF6.2.2    | **Scalability**<br/>Horizontally scale to handle high-frequency score updates and concurrent API/WebSocket clients.                                      |
| NF6.2.3    | **Reliability**<br/>Ensure zero data loss during service restarts; implement durable storage for scores and challenge definitions.                       |
| NF6.2.4    | **Security**<br/>Encrypt data in transit (TLS) and at rest; enforce strict RBAC for administrative actions.                                             |
| NF6.2.5    | **Maintainability**<br/>Modular code structure with separate packages for ingestion, computation, storage, API, and WebSocket layers.                    |
| NF6.2.6    | **Monitoring & Logging**<br/>Prometheus metrics (update processing time, API latency, WebSocket connections) and structured logs for auditing.           |
| NF6.2.7    | **Extensibility**<br/>Allow future integration with external reward platforms and gamification engines via pluggable adapters.                           |

## Data Model

```go
// pkg/models/gamification.go
package models

import "time"

// Score represents a normalized energy-efficiency score.
type Score struct {
    ZoneID    string    `json:"zoneId"`
    Timestamp time.Time `json:"timestamp"`
    Value     float64   `json:"value"`  // 0–100
}

// LeaderboardEntry holds one entry in a leaderboard.
type LeaderboardEntry struct {
    ZoneID    string  `json:"zoneId"`
    Score     float64 `json:"score"`
    Rank      int     `json:"rank"`
}

// Challenge defines a custom gamification challenge.
type Challenge struct {
    ChallengeID   string    `json:"challengeId"`
    Name          string    `json:"name"`
    Description   string    `json:"description"`
    Group         string    `json:"group"`        // e.g., floor-01
    StartTime     time.Time `json:"startTime"`
    EndTime       time.Time `json:"endTime"`
    BonusCriteria string    `json:"bonusCriteria"`
}

// RewardEvent represents a reward awarded to a zone or user.
type RewardEvent struct {
    EventID     string    `json:"eventId"`
    ZoneID      string    `json:"zoneId"`
    ChallengeID string    `json:"challengeId,omitempty"`
    Timestamp   time.Time `json:"timestamp"`
    Details     string    `json:"details"`
}
```

## API Specification

### REST Endpoints

```yaml
paths:
  /v1/gamification/leaderboards:
    get:
      summary: Get current leaderboards
      security:
        - BearerAuth: []
      parameters:
        - name: group
          in: query
          required: true
          schema: { type: string }
      responses:
        '200':
          description: List of LeaderboardEntry
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/LeaderboardEntry'

  /v1/gamification/scores/{zoneId}:
    get:
      summary: Get latest score for a zone
      security:
        - BearerAuth: []
      parameters:
        - name: zoneId
          in: path
          required: true
          schema: { type: string }
      responses:
        '200':
          description: Score for the zone
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Score'

  /v1/gamification/challenges:
    post:
      summary: Create a new challenge
      security:
        - BearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Challenge'
      responses:
        '201': { description: Created }

  /v1/gamification/challenges/{challengeId}:
    delete:
      summary: Delete a challenge
      security:
        - BearerAuth: []
      parameters:
        - name: challengeId
          in: path
          required: true
          schema: { type: string }
      responses:
        '204': { description: No Content }

  /v1/gamification/rewards:
    get:
      summary: List reward events
      security:
        - BearerAuth: []
      parameters:
        - name: zoneId
          in: query
          schema: { type: string }
      responses:
        '200':
          description: List of RewardEvent
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/RewardEvent'
```

### WebSocket

```
URL: wss://<host>/v1/gamification/stream
Subprotocol: Bearer <jwt-token>
Message format:
{
  "leaderboard": [ ...LeaderboardEntry ],
  "scoreUpdate": { ...Score },
  "rewardEvent": { ...RewardEvent }
}
```

## Processing Flow

1. **Score Ingestion**  
   - Subscribe to Analytics WebSocket or REST webhook to receive new `Score` objects.
2. **Leaderboard Update**  
   - Recompute rankings for affected group(s); update database.
3. **Reward Evaluation**  
   - Check active challenges; create `RewardEvent` if criteria met.
4. **Notification**  
   - Emit WebSocket messages for updates.

## Configuration & Deployment

| Key                          | Description                                               | Default             |
|------------------------------|-----------------------------------------------------------|---------------------|
| `GAM_PORT`                   | HTTP & WebSocket port                                     | `8085`              |
| `DB_CONN_STRING`             | Connection string for gamification database               | env var             |
| `WEBSOCKET_MAX_CLIENTS`      | Maximum concurrent WebSocket clients                      | `1000`              |
| `DEFAULT_LEADERBOARD_LIMIT`  | Default max entries returned                              | `50`                |
| `LOG_LEVEL`                  | `debug`, `info`, `warn`, `error`                          | `info`              |

## Future Enhancements

- Integration with external voucher/coupon systems  
- User-level gamification (per occupant vs. per zone)  
- Adaptive challenges based on usage patterns  
- RESTful pagination for leaderboards and events  

---
