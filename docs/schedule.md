
# NRG CHAMP – Re-Onboarding Schedule (Aug–Sep)

---

## 20/8 (1h30)
**Focus: Alignment on Re-Onboarding Findings**
- [J] Review repo status vs. re-onboarding doc, confirm gaps.
- [G] Summarize blockchain/ledger state & missing integrations.
- [A] Summarize infrastructure (Docker/K8s), API spec, and gamification skeleton status.
- Decide priorities: service integration first, then missing modules, then dashboards.  
  **Output:** Short plan doc in repo (`NEXT_STEPS.md`) + updated GitHub Project board.

---

## 24/8 (4h)
**Goal: Backend Integration Sprint 1**
- [G] Wire Device → Aggregator → MAPE Monitor/Analyze.
- [A] Ensure Docker-compose for local dev runs Aggregator + MAPE services; validate ingestion.
- [J] End-to-end test with simulated sensor data flowing into MAPE Monitor.  
  **Output:** First integrated pipeline Device → Aggregator → MAPE Monitor/Analyze.

---

## 25/8 (2h)
**Goal: Backend Integration Sprint 2**
- [G] Extend MAPE Plan/Execute with actuator stubs.
- [A] Expose aggregator + MAPE APIs in OpenAPI; check CORS/routing.  
  **Output:** Functional MAPE loop up to Execute, ready for connection to Ledger.

---

## 30/8 (4h)
**Goal: Ledger Service Integration**
- [G] Connect MAPE Execute outputs → Ledger service (transaction builder + retry).
- [A] Implement Ledger query API (`/transactions`, `/tx/{id}`), update OpenAPI.
- [J] Test flow Device → Aggregator → MAPE → Ledger (append-only store).  
  **Output:** Control commands and sensor data stored in ledger, retrievable via API.

---

## 31/8 (4h)
**Goal: Analytics & Gamification Sprint 1**
- [G] Implement Analytics KPIs: basic energy metrics (mocked calc).
- [A] Implement Gamification scaffolding: scoring rules, leaderboard DB schema.
- [J] Integrate ledger queries → analytics pipeline.  
  **Output:** Analytics & gamification back-end foundations in place.

---

## 6/9 (4h)
**Goal: Analytics & Gamification Sprint 2**
- [G] Extend Analytics: KPIs to scores. Wire into Gamification.
- [A] Implement leaderboard endpoints (`/leaderboard/public`, `/leaderboard/private`).
- [J] End-to-end test: simulated data → KPI → scores → leaderboard.  
  **Output:** Fully functional analytics + gamification APIs.

---

## 7/9 (4h)
**Goal: API Gateway & Docs**
- [G] Finalize OpenAPI spec across services; document blockchain + analytics.
- [A] Build API Gateway service (reverse proxy / central entrypoint).
- [J] Validate gateway routing; run Postman collection tests.  
  **Output:** Single entrypoint API Gateway + complete OpenAPI.

---

## 13/9 (4h)
**Goal: Frontend/Dashboard Sprint 1**
- [A] Scaffold React dashboard (Vite); basic layout + routes.
- [G] Provide API mocks and backend support (CORS/WebSocket if needed).
- [J] Connect dashboard to API Gateway for sensor data & leaderboards.  
  **Output:** Running frontend showing live data (charts + leaderboard placeholder).

---

## 14/9 (4h)
**Goal: Final Integration, QA & Demo Prep**
- [J] Full pipeline test: Device → Aggregator → MAPE → Ledger → Analytics → Gamification → API Gateway → Dashboard.
- [G] Write technical docs (ledger, MAPE, APIs, deployment).
- [A] Polish UI + Docker-compose/K8s manifests.  
  **Output:** End-to-end demo-ready MVP + docs + slides.

---

## Backup (5h, only if needed between 15/9–20/9)
**Use if major blockers appear in integration or front-end:**
- Debug broken flows (MAPE ↔ Ledger ↔ Analytics).
- Polish docs & CI/CD pipeline.
- Record demo video as fallback.

---

## Deliverables by 14/9
1. **Functional MVP**: Full data flow Device → Aggregator → MAPE → Ledger → Analytics/Gamification → Dashboard.
2. **Documentation**: Updated API specs, architecture diagrams, user + developer guides.
3. **Demo Materials**: Live dashboard or CLI, slides, demo script.
4. **Deployment Package**: Docker-compose/K8s manifests tested locally.