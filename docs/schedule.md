# NRG CHAMP — Service‑by‑Service Schedule (repo considered empty)

## 06/09 — 15:00–19:00 — Service 1: External Sensor Simulator + Device Adapter (code + deploy)

**Objective**  
Deliver the **external simulator** and the first internal microservice (**Device Adapter**) with **Kafka** integration. Both runnable via **docker‑compose** and have **Kubernetes manifests**.

**Scope & Deliverables**
- **External Sensor Simulator (standalone)**
    - Model: `T_in(t+Δt) = T_in(t) + α*(T_out − T_in) + β*HVAC_effect(t)`; parameters and initial values from `sim.properties`.
    - REST API: `GET /env` (T_in, T_out, hvac_state, zoneId); `POST /actuator` (HEAT_ON/OFF, COOL_ON/OFF, FAN level).
    - **Dockerfile + docker‑compose service** (for local dev). Optional K8s manifest to run in‑cluster for tests.
- **Device Adapter (internal)**
    - Publishes canonical readings to **Kafka** topic `device.readings` (JSON: `timestamp, zoneId, t_in, t_out, hvac_state`).
    - Optional debug endpoint: `GET /readings?zoneId=...` (last sample); plus `/health`.
    - **Dockerfile + docker‑compose service** (depends on Kafka).
    - **Kubernetes**: `k8s/device-adapter/` → `deployment.yaml`, `service.yaml`, `configmap.yaml` (connection/env), `kustomization.yaml`.
- **Kafka (dev)**
    - Compose: single‑broker **Kafka + Zookeeper**. Topic `device.readings` created on start.
    - K8s: single‑broker dev manifest (Bitnami single node) with topic CR or init‑job to create `device.readings`.
- **Verification**
    - End‑to‑end local: changes to actuator via simulator affect `T_in`; adapter publishes to `device.readings`.
    - `kubectl apply` brings up device‑adapter; logs show successful publish.

**Ownership**
- **Andrea**: docker‑compose, K8s manifests, adapter service & `/health`.
- **Giuseppe**: simulator model & properties; adapter producer; topic/bootstrap config; basic tests/logs.

---

## 07/09 — 09:00–13:00 — Service 2: Aggregator (code + deploy)

**Objective**  
Implement the **Aggregator** to **consume from Kafka** (`device.readings`), validate and buffer, and re‑expose via API and/or republish to a new topic for downstream services.

**Scope & Deliverables**
- **Consumer** from `device.readings`; **schema validation**; in‑memory time window buffer (e.g., last 15 min).
- API: `GET /latest?zoneId=...`, `GET /series?zoneId=...&from=&to=`, `/health`.
- **Dockerfile + compose** (depends on Kafka).
- **Kubernetes**: `k8s/aggregator/` → `deployment.yaml`, `service.yaml`, `configmap.yaml`, `kustomization.yaml`.
- **Verification**: device‑adapter → Kafka → aggregator consumer; `/latest` and `/series` return recent data; pod healthy.

**Ownership**
- **Andrea**: service skeleton, API, compose wiring, K8s manifests.
- **Giuseppe**: consumer, schema validation, windowing logic, tests/logs.

---

## 07/09 — 15:00–19:00 — Service 3 (Part 1): MAPE — Monitor & Analyze (code + deploy)

**Objective**  
Deliver **Monitor** and **Analyze** (comfort/anomaly checks).

**Scope & Deliverables**
- **Monitor**
    - Reads from Aggregator API (not Kafka).
    - API: `GET /status?zoneId=...`, `/health`.
- **Analyze**
    - Comfort band checks; basic anomaly flags (over/under‑shoot, oscillations).
- **Dockerfile + compose** (depends on Kafka).
- **Kubernetes**: `k8s/mape/monitor-analyze/` → `deployment.yaml`, `service.yaml`, `configmap.yaml`, `kustomization.yaml`.
- **Verification**: `/status?zoneId=Z1` responds with current metrics & flags; pods healthy; logs show consumption.

**Ownership**
- **Andrea**: monitor consumer, API & status endpoint, K8s manifests.
- **Giuseppe**: analyze rules, thresholds, tests/logs.

---

## 10/09 — 15:00–19:00 — Service 3 (Part 2): MAPE — Plan & Execute (code + deploy)

**Objective**  
Complete **Plan** (rule‑based) and **Execute** (actuator commands back to the simulator via Device Adapter).

**Scope & Deliverables**
- **Plan**: simple rules (e.g., if `t_in > T_high` → COOL_ON to target; if `t_in < T_low` → HEAT_ON).
- **Execute**: call Device Adapter → Simulator `POST /actuator` with retry/ACK‑NACK handling and idempotency key.
- API: `POST /actions/test` (dry‑run plan for a snapshot), `GET /last-actions`, `/health`.
- **Dockerfile + compose**.
- **Kubernetes**: `k8s/mape/plan-execute/` → `deployment.yaml`, `service.yaml`, `configmap.yaml`, `kustomization.yaml`.
- **Verification**: closed loop — commands change HVAC state; simulator `T_in` responds; MAPE logs actions.

**Ownership**
- **Giuseppe**: planning rules, dry‑run endpoint, retries/idempotency, tests.
- **Andrea**: execute client & endpoints, compose & K8s wiring.

---

## 13/09 — 09:00–13:00 — Service 4: Ledger (code + deploy)

**Objective**  
Persist events (selected readings, MAPE decisions/actions) into an **append‑only ledger**, exposing query APIs.

**Scope & Deliverables**
- Storage: append‑only store (file/embedded DB) with integrity (checksum or hash chain).
- API: `POST /events` (writes from Aggregator/MAPE), `GET /events?type=&zoneId=&from=&to=&page=&size=`, `GET /events/{id}`, `/health`.
- Ingestion from services via HTTP (or Kafka sink optional later). **This becomes the single source of truth** for higher layers.
- **Dockerfile + compose**.
- **Kubernetes**: `k8s/ledger/` → `deployment.yaml`, `service.yaml`, `configmap.yaml`, `kustomization.yaml`.
- **Verification**: writes succeed with correlation IDs; pagination works; integrity check passes for a sample chain.

**Ownership**
- **Giuseppe**: event model & integrity chain, write path, tests.
- **Andrea**: query endpoints, pagination/filters, compose + K8s integration.

---

## 13/09 — 15:00–19:00 — Service 5: Assessment (code + deploy; **reads from Ledger only**)

**Objective**  
Compute **KPIs** strictly from the **Ledger** (no other upstream dependency) and expose them for consumers.

**Scope & Deliverables**
- KPIs: comfort‑time %, anomaly counts, mean deviation from target, actuator on‑time % (rivedere, anche per gamification).
- API: `GET /kpi/summary?zoneId=&from=&to=`, `GET /kpi/series?metric=&zoneId=&from=&to=`, `/health`.
- Data source: **Ledger only** (HTTP queries); internal cache for recent windows.
- **Dockerfile + compose**.
- **Kubernetes**: `k8s/assessment/` → `deployment.yaml`, `service.yaml`, `configmap.yaml`, `kustomization.yaml`.
- **Verification**: KPIs reflect ledger content; time‑window filters work; service scales independently.

**Ownership**
- **Andrea**: API & caching; compose/K8s wiring.
- **Giuseppe**: KPI formulas & validation; tests.

---

## 14/09 — 09:00–13:00 — Service 6: Gamification (code + deploy; **reads from Ledger only**)

**Objective**  
Compute **scores and leaderboards** strictly from the **Ledger**; fully independent from Assessment service.

**Scope & Deliverables**
- Scoring: weights on ledger‑derived KPIs/events (e.g., comfort %, energy‑proxy events, anomaly penalties) computed **internally**.
- API: `POST /score/recompute?zoneId=&from=&to=`, `GET /leaderboard?scope=building|floor|zone&page=&size=`, `/health`.
- Data source: **Ledger only** (HTTP queries); maintain scores store (file/embedded DB).
- **Dockerfile + compose**.
- **Kubernetes**: `k8s/gamification/` → `deployment.yaml`, `service.yaml`, `configmap.yaml`, `kustomization.yaml`.
- **Verification**: recompute produces deterministic scores; leaderboard paging/sorting works; independent scaling verified.

**Ownership**
- **Giuseppe**: scoring design + recompute endpoint + tests.
- **Andrea**: leaderboard endpoints + storage model + compose/K8s integration.

---

## 14/09 — 15:00–19:00 — System Hardening, Docs & Demo (compose + Kubernetes)

**Objective**  
End‑to‑end validation and packaging for **both docker‑compose and Kubernetes**.

**Scope & Deliverables**
- **E2E tests**: full chain **Simulator → Device Adapter → Kafka → Aggregator → MAPE (M/A/P/E) → Ledger → (Assessment || Gamification)**; inject faults (Kafka outage, actuator NACKs, network blips).
- **Compose UX**: one‑command `docker-compose up` for the full stack; `.env` defaults; sample `sim.properties`.
- **Kubernetes**: per‑service `deployment.yaml`, `service.yaml`, `configmap.yaml`, `kustomization.yaml`; root `k8s/` README with apply order; optional `kustomize build` or namespace overlay.
- **Docs**: service READMEs, OpenAPI specs, architecture diagram, run‑book (compose + K8s), topic & schema reference.
- **Demo**: scripted flows (heat wave/cold snap) showing closed‑loop control and score swings.

**Ownership**
- **Andrea**: compose orchestration, K8s apply order & README, demo scripts.
- **Giuseppe**: OpenAPI, architecture doc, fault‑injection tests, schema/topic doc.

---

### General notes
- All services ship with **health/readiness endpoints**, **structured logging**, and **basic tests**.
- Security (JWT/RBAC) and an API Gateway can be added in the next iteration; priority here is **service‑by‑service delivery with compose + K8s**.
- Message schemas (JSON) and Kafka topics are documented and versioned; breaking changes require explicit migration notes.
