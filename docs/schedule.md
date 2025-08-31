# NRG CHAMP — 8×4h Rescheduled Roadmap (starting after Meeting #1)

**Legend:**
- **[G]** Giuseppe (AI prompting + blockchain/ledger + backend/docs)
- **[A]** Andrea (frontend + Docker-compose + backend integration + API)
- **[J]** Joint/pair time

---

## Meeting 1 — Backend Integration Sprint 1 (4h) - 31/08, 9:00-13:00
**Goal:** Device → Aggregator → MAPE Monitor/Analyze wired and running locally.

- **[G]** Wire **Device → Aggregator** ingestion path; enforce payload validation & buffering, add structured logging.
- **[A]** Stand up **docker-compose** stack for **Aggregator + MAPE Monitor**; ensure live logs & hot-reload where possible.
- **[J]** Connect **MAPE Monitor** to Aggregator stream; implement first pass of **Analyze** (baseline/anomaly heuristic).
- **Hardening:** Create a minimal **health/ready** endpoint for Aggregator & MAPE Monitor; add smoke tests (curl/Postman).
- **Output/AC:** JSON measurements flow into MAPE Monitor; Analyze emits basic signals; `docker-compose up` works end-to-end.

---

## Meeting 2 — Backend Integration Sprint 2 (4h) - 06/08, 9:00-13:00
**Goal:** Complete **Plan/Execute** + API polish (expanded from the old 2h session).

- **[G]** Implement **MAPE Plan** (simple rule-based: setpoint deltas, fan levels) with guardrails; write unit tests for rules.
- **[A]** Implement **MAPE Execute** → actuator stub **(REST/MQTT)** with ACK/NACK handling and retry/backoff.
- **[J]** Flesh out **OpenAPI** for Aggregator/MAPE endpoints; enable **CORS** and auth placeholders (JWT claim stub).
- **Output/AC:** `POST /hvac/commands` and `GET /hvac/status` live; full MAPE loop runs on local stack; OpenAPI synced to code.

---

## Meeting 3 — Ledger Service Integration (4h) - 06/08, 15:00-19:00
**Goal:** Persist control actions & selected sensor snapshots to ledger and expose queries.

- **[G]** Wire **MAPE Execute → Ledger** (transaction builder, append-only store, retry cache), add idempotency key.
- **[A]** Implement **Ledger Query API** (`/transactions`, `/transactions/{id}`, pagination/filters) + OpenAPI updates.
- **[J]** E2E test: Device → Aggregator → MAPE → **Ledger**; verify persisted records + consistent timestamps.
- **Output/AC:** Transactions are created on actions, retrievable via API; retry works; ledger passes basic integrity checks.

---

## Meeting 4 — Analytics & Gamification Sprint 1 (4h) - 07/08, 09:00-13:00
**Goal:** Stand up KPI pipeline and gamification scaffold.

- **[G]** Implement **Analytics KPIs** (e.g., avg temp deviation, control efficiency proxy) pulling from ledger & recent readings.
- **[A]** Scaffold **Gamification** service: scoring rules interface, **leaderboard DB schema**, service skeleton and routes.
- **[J]** Integrate **Analytics → Gamification** (scores computed from KPIs); add fixtures for demo floors/zones.
- **Output/AC:** `/analytics/kpi` returns values; `/gamification/score` and basic `/leaderboard` routes exist with sample data.

---

## Meeting 5 — Analytics & Gamification Sprint 2 (4h) - 10/08, 15:00-19:00
**Goal:** Productionize endpoints & E2E scoring.

- **[A]** Implement **leaderboard endpoints** (public/private) with scopes; add paging/sorting; tighten input validation.
- **[G]** Extend Analytics to **score computation** (weights, normalization); persist score events; unit tests for formulas.
- **[J]** E2E test: simulated data → KPI → score → leaderboard; add error paths for missing data/windows.
- **Output/AC:** Stable APIs for KPIs & leaderboards; tests for scoring edge cases; telemetry/log lines cover full flow.

---

## Meeting 6 — API Gateway & Documentation (4h) - 13/08, 09:00-13:00
**Goal:** One entry point + consistent contracts + tested with a collection.

- **[A]** Implement **API Gateway** (reverse proxy/routing); map `/api/*` to services; configure CORS & rate-limit defaults.
- **[G]** **Unify OpenAPI** across services (refs/components), add examples; generate a **Postman** (or Bruno) collection.
- **[J]** Run the collection against the gateway; fix any contract drift; document **auth story** (JWT placeholder, RBAC draft).
- **Output/AC:** Single base URL for the system; spec + collection pass green; minimal security posture documented.

---

## Meeting 7 — Frontend/Dashboard Sprint 1 (4h) - 13/08, 15:00-19:00
**Goal:** Live dashboard for sensor stream, MAPE status, KPIs, and leaderboard.

- **[A]** Scaffold **React (Vite)** app: routes + layout; build **SensorChart**, **MAPEStatus**, **KPICards**, **Leaderboard**; wire to Gateway.
- **[G]** Provide **mock servers** (if needed) and sample payloads; help with query shapes and error states; refine gateway headers.
- **[J]** UX passes for loading/empty/error; add minimal theming; confirm mobile-safe layout for demo.
- **Output/AC:** Dashboard shows live data (charts, status, leaderboard); all calls via Gateway; basic error handling in UI.

---

## Meeting 8 — Final Integration, QA, & Demo Prep (4h) - 14/08, 09:00-13:00
**Goal:** End-to-end hardening + delivery materials.

- **[J]** Full pipeline test: Device → Aggregator → MAPE → Ledger → Analytics/Gamification → Gateway → Dashboard; introduce faults (sensor gap, NACK, ledger delay) and verify resilience/logging.
- **[G]** Finalize **technical docs** (architecture diagrams, MAPE decisions, ledger model, APIs, deployment notes).
- **[A]** Polish **docker-compose** UX (one-command up), tweak UI, export a ready **Postman collection**; optional: lint/build CI draft.
- **Output/AC:** Demo-ready MVP + docs + slides/script; reproducible local run; known issues list & next-steps backlog.
