# NRG CHAMP — Updated Schedule (based on uploaded docs & schedule)

**Legend:** [G] Giuseppe · [A] Andrea · [J] Joint

---

## Meeting 1 — Backend Integration Sprint 1 — **06/09, 9:00-13:00**
**Goal:** See bullets below.

- [A] Bring up docker-compose with Aggregator + MAPE Monitor + message queue; add metrics.
- [G] Enforce JSON schema validation & windowing in Aggregator.
- [J] Wire Monitor to MQ topic; add /health; verify Prometheus.

**Acceptance:** Short demo + logs + endpoints validated.


## Meeting 2 — Backend Integration Sprint 2 — **06/09, 15:00-19:00**
**Goal:** See bullets below.

- [G] Implement MAPE Plan (rule-based) + unit tests.
- [A] Implement Execute to actuator stub (REST/MQTT) with ACK/NACK + retry.
- [J] OpenAPI for Aggregator/MAPE; enable CORS and JWT placeholder (RBAC to follow).

**Acceptance:** Short demo + logs + endpoints validated.


## Meeting 3 — Ledger Service Integration — **07/08, 09:00-13:00**
**Goal:** See bullets below.

- [G] Transaction Builder + Local Ledger + retry cache + idempotency.
- [A] Implement Ledger Query API (`/transactions`, `/transactions/{id}`) with pagination/filters; update OpenAPI.
- [J] E2E test MAPE→Ledger (timestamps & integrity).

**Acceptance:** Short demo + logs + endpoints validated.


## Meeting 4 — Analytics & Gamification Sprint 1 — **07/08, 15:00-19:00**
**Goal:** See bullets below.

- [G] Implement KPIs from ledger/recent readings.
- [A] Gamification scaffold: rules interface + leaderboard schema.
- [J] Integrate Analytics→Gamification; seed demo fixtures.

**Acceptance:** Short demo + logs + endpoints validated.


## Meeting 5 — Analytics & Gamification Sprint 2 — **10/08, 15:00-19:00**
**Goal:** See bullets below.

- [A] Leaderboard endpoints (public/private) with paging/sorting/validation.
- [G] Scoring (weights/normalization), persist score events + unit tests.
- [J] E2E: data→KPI→score→leaderboard; error-path tests.

**Acceptance:** Short demo + logs + endpoints validated.


## Meeting 6 — API Gateway & Documentation — **13/08, 09:00-13:00**
**Goal:** See bullets below.

- [A] Implement API Gateway routing `/api/*`; CORS + rate-limits.
- [G] Unify OpenAPI; generate collection; document JWT roles (auditor/admin) and route protection.
- [J] Run collection through Gateway; fix contract drift; write security notes.

**Acceptance:** Short demo + logs + endpoints validated.


## Meeting 7 — Frontend/Dashboard Sprint 1 — **13/08, 15:00-19:00**
**Goal:** See bullets below.

- [A] Scaffold React (Vite) app; components: SensorChart, MAPEStatus, KPICards, Leaderboard.
- [G] Provide mocks/payloads; ensure gateway headers; align `/api/v1/*` calls.
- [J] UX passes: loading/empty/error; basic theming; mobile-friendly.

**Acceptance:** Short demo + logs + endpoints validated.


## Meeting 8 — Final Integration, QA & Demo Prep — **14/08, 09:00-13:00**
**Goal:** See bullets below.

- [J] Full pipeline test with injected faults (network down, NACKs, ledger delay).
- [G] Finalize technical docs (architecture, MAPE, ledger, APIs, deployment).
- [A] Polish docker-compose UX (one-command up); export Postman collection; optional CI draft.

**Acceptance:** Short demo + logs + endpoints validated.
