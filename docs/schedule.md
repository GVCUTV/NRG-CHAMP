## NRG CHAMP Project Schedule
## Detailed 6-Week, 12-Session Schedule

**Notation:**
- [A] = Andrea
- [G] = Giuseppe
- [J] = Joint (both together, strongly paired)

---

### WEEK 1

#### Session 1 (28/7/2025, 2h, Joint): Project Recap, Audit & Planning
- [G & A] Recap what is done:
    - Audit the repo: List all working, stub, unfinished, and outdated parts (backend, API, blockchain, analytics, CI/CD, Docker, UI).
    - Review existing docs: Mark what can be reused, what needs rewriting.
    - Recap external system contract (API/payload, endpoints): ensure up-to-date.
- **Output:**
    - Shared doc: "NRG CHAMP Current Status & Gaps" (list of modules/files, status, todos).
    - Updated task board for next sessions.

#### Session 2 (30/7/2025, 3h, Joint): Architecture & Codebase Re-alignment
- [A] Begin updating Docker-compose and environment config to match actual modules/structure.
- [G] Start updating/re-drafting the architecture diagrams (module, data flow, API, blockchain, UI).
- [J] Both review and decide which code/modules need rewriting, which just cleanup, and which are fine.
- **Output:**
    - Cleaned-up repo structure.
    - Updated high-level architecture diagram and technical debt list.

---

### WEEK 2

#### Session 3 (4/8/2025, 2h, Joint): Documentation and Module Specification Refresh
- [G] Update/refresh module specs and API contracts (focus: blockchain, data aggregator, MAPE).
- [A] Update/refresh specs for front-end/CLI, analytics, gamification.
- [J] Both update the documentation index, plan for live collaborative docs.
- **Output:**
    - Current, synced specs for each major module.
    - Doc index (what's needed, what's reusable).

#### Session 4 (6/8/2025, 3h): Refactor & Re-implement (Backend)
- [G] Focus: Data Aggregator, MAPE (monitor/analyze), ensure up-to-date with external system interface.
- [A] Focus: Analytics/gamification backend, align with latest APIs; stub out/test endpoints if needed.
- **Joint last hour:**
    - Quick demo/test: Data flow from ingestion → MAPE → analytics.
- **Output:**
    - Cleaned and working backend pipeline for sensor → MAPE → analytics.

---

### WEEK 3

#### Session 5 (11/8/2025, 2h): Blockchain & Control Path Refactor
- [G] Refactor blockchain transaction archiving and local ledger if needed; update/expand API tests.
- [A] Review/update actuator control path (MAPE Execute), ensure API matches new contract, basic unit tests.
- **Output:**
    - Blockchain archiving and control APIs working/up-to-date.

#### Session 6 (13/8/2025, 3h, Joint): Front-End/CLI Integration & Testing
- [A] Update and connect front-end (React) or CLI (if you go CLI route) to current APIs; quick UI improvements.
- [G] Update API mocks/docs for front-end testing; verify end-to-end flow for key use cases.
- **Joint:**
    - Live test: sensor data → backend → UI/CLI; check for bugs/data mismatches.
- **Output:**
    - End-to-end “happy path” working; UI or CLI presents real (not just stubbed) data.

---

### WEEK 4

#### Session 7 (20/8/2025, 2h): CI/CD & Environment Hardening
- [A] Refactor and harden Docker-compose/dev scripts for full local environment, validate with new codebase.
- [G] Update CI/CD config (test, lint, deploy stages); add missing test automation as possible.
- **Joint:**
    - Test deploy from scratch; doc any friction or issues for final doc.
- **Output:**
    - Fully working dev env, reproducible builds, CI/CD tested at least locally.

#### Session 8 (20/8/2025, 3h, Joint): Error Paths & Edge Cases
- [G] Test edge cases: network loss, actuator NACK, blockchain unavailability; expand logging, monitoring.
- [A] UI/CLI: error feedback, loading states, missing data cases.
- **Joint:**
    - Log and triage new bugs; assign fixes.
- **Output:**
    - System resilient to failures; bugs and fixes tracked.

---

### WEEK 5

#### Session 9 (25/8/2025, 2h): Analytics & Gamification Polish
- [G] KPI/calculation checks, scoring logic tuning, edge cases (e.g. missing data).
- [A] UI/CLI: polish leaderboard display, gamification feedback, and data fetch.
- **Output:**
    - Analytics/gamification features ready for MVP.

#### Session 10 (27/8/2025, 3h, Joint): Documentation & Review
- [J] Write/review all user, deployment, developer, and API documentation.
- [G] Focus: blockchain, backend, API docs, security.
- [A] Focus: UI/CLI, deployment, Docker, test/usage guide.
- **Output:**
    - All documentation for demo, hand-in, or deploy.

---

### WEEK 6

#### Session 11 (1/9/2025, 2h, Joint): Final Integration, Testing, & Fixes
- [J] Complete integration test of the whole system; live demo run-through; last bug-fixes as needed.
- **Output:**
    - All modules connected, system works as a whole.

#### Session 12 (3/9/2025, 3h, Joint): Demo & Delivery Prep
- [G] Prepare technical presentation, demo script.
- [A] Polish UI/CLI for demo, create user walk-through.
- [J] Run final dry-run of demo; record (if needed); Q&A prep.
- **Output:**
    - Demo-ready system, slides, walkthrough video/script, all code and docs ready for submission/delivery.
