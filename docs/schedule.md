## 06/09 — 15:00–19:00 — Service 1: Device (External Simulator + Device Adapter)

**Objective:** Deliver the **external sensor simulator** (outside NRG CHAMP) and the **NRG CHAMP Device Adapter** (first internal service), both coded and deployed via docker‑compose.

**Scope & Deliverables**
- **External Sensor Simulator (standalone project):**
    - Implements indoor temperature evolution: `T_in(t+Δt) = f(T_in(t), T_out(t), HVAC_state(t))` (cooling/heating effects, simple first‑order model).
    - **Inputs:** properties file with initial `T_in0`, `T_out0`, model coefficients (cooling/heating rates, thermal inertia).
    - **API:** `GET /env` (current T_in, T_out, hvac state); `POST /actuator` (commands: HEAT_ON/OFF, COOL_ON/OFF, FAN levels).
- **NRG CHAMP Device Adapter (internal microservice):**
    - Pulls/polls simulator readings; normalizes to **canonical JSON** schema.
    - Exposes **`GET /readings`** for current sample; **`/health`** for readiness; structured logging.
    - **Dockerfiles** for both simulator and adapter; **docker‑compose** stack up & running.
- **Verification:** local run shows evolving T_in responding to actuator commands.

**Ownership**
- **Andrea:** docker‑compose, containerization, adapter API and health endpoints.
- **Giuseppe:** simulator math/model, properties file format, command API, logs/tests.

---

## 07/09 — 09:00–13:00 — Service 2: Aggregator (code + deploy)

**Objective:** Implement and deploy the **Aggregator** to ingest device‑adapter data, validate, buffer, and expose it downstream.

**Scope & Deliverables**
- **API:** `POST /ingest` (canonical reading), `GET /latest?zoneId=…`, `GET /series?from=&to=`; `/health`.
- **Validation & Buffering:** JSON schema validation; in‑memory ring buffer; simple time windowing.
- **Integration:** wire the Device Adapter → Aggregator via HTTP.
- **Deploy:** Dockerfile + compose; logs/metrics enabled.
- **Verification:** end‑to‑end flow: Simulator → Device Adapter → Aggregator (latest/series endpoints return data).

**Ownership**
- **Andrea:** service skeleton, endpoints, compose wiring.
- **Giuseppe:** schema validation, buffering/windowing logic, tests/logs.

---

## 07/09 — 15:00–19:00 — Service 3 (Part 1): MAPE — Monitor & Analyze (code + deploy)

**Objective:** Deliver a first MAPE service focusing on **Monitor** (subscribe/pull from Aggregator) and **Analyze** (baseline + simple anomaly/comfort detection).

**Scope & Deliverables**
- **API:** `GET /status` (per zone), `/health`.
- **Monitor:** periodic pull from Aggregator; maintain sliding state (avg, min/max, trend).
- **Analyze:** comfort band check, simple anomaly flags (e.g., overheating, oscillation).
- **Deploy:** Dockerfile + compose; structured logs.
- **Verification:** Analyzer status reflects live data trends; health passes.

**Ownership**
- **Andrea:** monitor pulling & state.
- **Giuseppe:** analyze rules & thresholds; tests.

---

## 10/09 — 15:00–19:00 — Service 3 (Part 2): MAPE — Plan & Execute (code + deploy)

**Objective:** Complete MAPE with **Plan** (simple rule‑based policy) and **Execute** (actuator commands back to simulator via the Device Adapter).

**Scope & Deliverables**
- **Plan:** rule set (e.g., if T_in > T_high → COOL_ON to target; if T_in < T_low → HEAT_ON).
- **Execute:** call Device Adapter → Simulator `POST /actuator` with retry/backoff and ACK/NACK handling.
- **API:** `POST /actions/test` (dry‑run plan for a snapshot), `GET /last-actions`.
- **Deploy:** Dockerfile + compose; logs + basic alerting (failed command attempts).
- **Verification:** closed loop — commands change HVAC state, simulator T_in responds as expected.

**Ownership**
- **Giuseppe:** planning rules, dry‑run endpoint, logging & retries.
- **Andrea:** execute client, integration & compose, action endpoints.

---

## 13/09 — 09:00–13:00 — Service 4: Ledger (code + deploy)

**Objective:** Create a **Ledger** microservice to persist key events (readings snapshots, MAPE decisions, actuator commands).

**Scope & Deliverables**
- **Storage:** append‑only local store (file/embedded DB) with integrity checks (e.g., checksum or hash chain).
- **API:** `POST /events` (MAPE/agg record), `GET /events?type=&from=&to=&page=&size=`, `GET /events/{id}`; `/health`.
- **Integration:** MAPE and Aggregator post events; include correlation IDs & timestamps.
- **Deploy:** Dockerfile + compose; retention policy placeholder.
- **Verification:** events stream is persisted and queryable; IDs are stable; basic pagination works.

**Ownership**
- **Giuseppe:** event model, integrity (checksum/hash chain), write path & tests.
- **Andrea:** read/query endpoints, pagination/filters, compose integration.

---

## 13/09 — 15:00–19:00 — Service 5: Assessment (code + deploy)

**Objective:** Build **Assessment** to compute KPIs from the Ledger/Aggregator and expose them for downstream use.

**Scope & Deliverables**
- **KPIs:** comfort‑time %, anomaly counts, average deviation from target, actuator on‑time %.
- **API:** `GET /kpi/summary?zoneId=&from=&to=`, `GET /kpi/series?metric=&from=&to=`; `/health`.
- **Integration:** reads from Ledger and Aggregator; caches recent windows.
- **Deploy:** Dockerfile + compose; logs/tests.
- **Verification:** KPIs reflect live runs and react to control policies.

**Ownership**
- **Andrea:** API & caching; compose wiring.
- **Giuseppe:** KPI formulas & validation; tests.

---

## 14/09 — 09:00–13:00 — Service 6: Gamification (code + deploy)

**Objective:** Implement **Gamification** consuming Assessment to produce scores and leaderboards.

**Scope & Deliverables**
- **Scoring rules:** weights on KPIs (e.g., comfort %, energy proxy, anomaly penalty).
- **API:** `POST /score/recompute?zoneId=&from=&to=`, `GET /leaderboard?scope=building|floor|zone&page=&size=`; `/health`.
- **Storage:** simple table/file for scores & snapshots.
- **Deploy:** Dockerfile + compose.
- **Verification:** recompute produces scores; leaderboard lists entities with deterministic ordering.

**Ownership**
- **Giuseppe:** scoring design + recompute endpoint + tests.
- **Andrea:** leaderboard endpoints + storage model + compose integration.

---

## 14/09 — 15:00–19:00 — System Hardening, Docs & Demo

**Objective:** End‑to‑end validation and packaging.

**Scope & Deliverables**
- **E2E tests:** full chain **Simulator → Device Adapter → Aggregator → MAPE (M/A/P/E) → Ledger → Assessment → Gamification** with fault injection (network blips, actuator NACKs).
- **Docs:** service READMEs, properties file reference, OpenAPI specs, architecture diagram, run‑book.
- **DX:** one‑command `docker-compose up` for the entire stack; sample `.properties` and sample datasets.
- **Demo:** scripted flows (heat wave, cold snap) showing closed‑loop control and score changes.

**Ownership**
- **Andrea:** compose orchestration, run‑book, demo scripts.
- **Giuseppe:** OpenAPI, architecture doc, fault‑injection tests, properties reference.
