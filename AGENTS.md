// v0
// AGENTS.md
# AGENTS.md — Codex Operational Rules for NRG CHAMP

> **Purpose:**  
> These rules define how **Codex** must operate when performing automated coding tasks in the NRG CHAMP project.  
> Codex acts as the **implementation and integration agent**, executing precise prompts provided by GPT or the user.  
> Codex produces **code, diffs, PRs, and documentation updates** — not plans or high-level designs.

---

## 1) General compliance & scope discipline

- Always obey **all rules** below unless explicitly instructed otherwise.
- **Modify only** what the prompt explicitly defines. Never alter unrelated files or code.
- If you must adjust surrounding code to make the project compile or run, keep the modification **minimal** and document it in your PR.
- If a rule is broken for valid reasons, **report it** in the PR body or summary:
    - which rule was broken,
    - how, and
    - why the exception was necessary.

---

## 2) Output & delivery expectations

- **Default output:** A **branch + pull request (PR)** with a clear **Git diff** and descriptive commit messages.
- Include a section titled **“Diff Summary & Notes”** in the PR body explaining the main changes in human-readable form.
- Keep changes atomic — prefer several small PRs over one large PR.
- For local deployment, code must be **ready-to-drop** (fully replaceable without manual editing).

---

## 3) File completeness & version headers

- Each created or modified file must start with:
  ```go
  // vN        
  //increment previous version, or start at v0 if unknown
  // path/to/file.go
  ```
- Always generate **complete files**, preserving code not intended to be modified.
- The output must be **production-ready** and buildable as-is.

---

## 4) Commenting & design rationale

- Code must be **fully commented** in natural, human-like style (never mention AI).
- At the end of the PR body, include a short **Design Rationale** section describing:
    - Why this approach was chosen
    - Any trade-offs or limitations considered

---

## 5) Logging policy

- Every functional operation in the code must log to **both** stdout **and** a logfile.
- Use **Go’s `log/slog`** exclusively.
- Do not use alternative logging libraries unless the user explicitly allows it.

---

## 6) Library & dependency policy

- Use **Go standard libraries only** unless explicitly instructed otherwise.
- If a library outside the stdlib is required, **pause execution** and open a PR comment explaining the need and suggesting alternatives.

---

## 7) Docker, build & runtime policy

- **Dockerfiles** must follow this convention:
    - **Build stage:** `golang:1.23-alpine`
    - **Runtime stage:** `alpine:3.20`
- If a health-check endpoint is required, implement it at `GET /health` returning HTTP 200 with a minimal JSON or text payload.

---

## 8) HTTP endpoint discipline

- Each endpoint must accept **only** the correct HTTP method:
    - `GET` for reads/status
    - `POST` for create/execute commands
    - `PUT` / `PATCH` for configuration updates
    - `DELETE` for deletions
- Reject all other methods with appropriate status codes.

---

## 9) Definition of Done (DoD)

A Codex task is considered **complete** only if all of the following hold:

1. The **build succeeds** without errors.
2. The PR is **limited to scope** with no unrelated changes.
3. Code is **fully commented** and follows project conventions.
4. Logging is implemented and functional.
5. Documentation or rationale conflicts (if any) are flagged and approved.

---

## 10) Repository synchronization

- Operate on the **snapshot** of the assigned branch at task start.
- Before writing code, ensure the environment is synced to the latest `origin/<branch>` commit.
- Include the current **HEAD commit hash** in the PR description.
- If the branch changes upstream during task execution, **stop** and request a restart.

---

## 11) Documentation awareness

- Before editing or adding code, **read** the relevant documentation:
    - `docs/project_documentation.md`
    - `docs/ragionamenti.md`
    - `docs/relazione.pdf`
    - any relevant subfile under `docs/`
- If proposed changes **conflict** with documentation, Codex must:
    1. Flag a **“Design Conflict”** in the PR.
    2. Request confirmation from the user before proceeding further.
- Never override documentation assumptions silently.

---

## 12) Rule violation & conflict reporting

- If Codex cannot satisfy all constraints due to limitations (e.g., missing stdlib feature, dependency need, conflicting rule), it must:
    1. Stop and describe the conflict in PR comments.
    2. Suggest possible resolutions or alternatives.

---

## 13) Atomicity & granularity of changes

- Large changes must be split into smaller, reviewable steps (separate PRs).
- Each PR should handle **one logical modification** only — e.g., “add logging”, “fix HTTP method validation”, “refactor config parser”.

---

## 14) Version control & commit conventions

- **PR title format:** `[Service|Module] — <short action>: <goal>`  
  Example: `aggregator — add slog and /health handler`
- **PR body must include:**
    - Diff Summary & Notes
    - Design Rationale
    - Impacts, risks, migration notes (if any)
- **Commit messages:** imperative style (e.g., “Add logging middleware”), one logical change per commit.

---

## 15) Limitations & escalation

- If a task cannot be completed within current Codex capabilities (e.g., missing environment tools, blocked dependency, ambiguous design), Codex must:
    - Halt the task,
    - State the limitation explicitly, and
    - Suggest how the user or GPT could revise the prompt.

---

## 16) Behavioral checklist

-  Always operate on the latest repo snapshot.
-  Ensure the build passes before PR completion.
-  Always use `log/slog` for logging.
-  Always follow Docker base images (`golang:1.23-alpine`, `alpine:3.20`).
-  Comment and document every significant code section.
-  Produce clear, human-readable PRs and commit messages.
-  Never edit beyond scope or modify unrelated code.
-  Never use non-standard libraries without explicit permission.
-  Never silently ignore contradictions with project documentation.

---

## 17) Pull Request rules
- Always open pull requests on the existing branch named `test`.
- Do **not** open pull requests on existing branch named `main`.
- 

---

### 18) Docker & Compose Pre‑flight Rules

- **Build context discipline**: For monorepo local modules (e.g., `circuit_breaker/`), set each service’s Compose `build.context` to the **repo root**, and set `build.dockerfile` to the per‑service Dockerfile path (e.g., `services/ledger/Dockerfile`). This guarantees local shared modules are copyable during image builds.
- **Go workspace discipline**: The repository root `go.work` is **mandatory**. It MUST list `./circuit_breaker`, `./services/aggregator`, `./services/assessment`, `./services/gamification`, `./services/ledger`, `./services/mape`, `./services/topic-init`, and `./zone_simulator`. Docker builds copy this file to `/src/go.work`, and `go work sync` MUST run whenever module dependencies change so that `go.work.sum` stays current.
- **Local module replace**: Avoid absolute paths. Prefer workspace resolution; if a `replace` is unavoidable, use a relative path that remains valid once the repo is copied to `/src` inside the builder.
- **Dockerfile copies**: For services depending on local modules, the Dockerfile **must** include:
    - `COPY circuit_breaker /circuit_breaker` (or the correct module folder);
    - `COPY services/<svc>/go.mod services/<svc>/go.sum ./` (for dependency resolution cache);
    - `COPY services/<svc>/ ./` (the service sources).
- **Unique stage names**: Ensure all multi‑stage `FROM ... AS <name>` identifiers are **unique** within a Dockerfile. Update all `COPY --from=<name>` references accordingly.
- **Pre‑merge checks (mandatory for every PR that touches Docker/Go modules)**:
    1. `docker compose build <service>` succeeds for all changed services.
    2. `docker compose up -d <service>` starts without crash‑looping.
    3. `go mod tidy` has been executed and any changes committed whenever `go.mod`/`go.sum` are edited.
    4. No linter warnings such as **DuplicateStageName** or unresolved `replace` directives.
    5. If a non‑standard `replace` is used, explain it briefly in the PR body.

**Violation reporting**:  
If Codex must deviate from any rule (e.g., CI limitations), it must **explicitly report** the deviation **at the top of the PR body**, stating which rule, why it was broken, and how risk is mitigated.

---

**End of AGENTS.md — Codex Operational Rules**
