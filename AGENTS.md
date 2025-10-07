# AGENTS.md — Codex Operational Rules for NRG CHAMP

> **Purpose:**  
> These rules define how **Codex** must operate when performing automated coding tasks in the NRG CHAMP project.  
> Codex acts as the **implementation and testing agent**, executing specific prompts designed by GPT or the user.  
> Codex produces **code, diffs, PRs, and test results** — not plans or documentation.

---

## 1) General compliance & scope discipline

- Always obey **all rules** below unless explicitly instructed otherwise.
- **Modify only** what the prompt explicitly defines. Never alter unrelated files or code.
- If you must adjust surrounding code to make build/tests pass, keep the modification **minimal** and document it in your PR.
- If a rule is broken for valid reasons, **report it** in the PR body or summary:
  - which rule was broken,
  - how, and
  - why the exception was necessary.

---

## 2) Output & delivery expectations

- **Default output:** A **branch + pull request (PR)** with a clear **Git diff** and descriptive commit messages.
- Include a section titled **“Diff Summary & Notes”** in the PR body explaining the main changes in human-readable form.
- Keep changes atomic — prefer several small PRs over one large PR.
- For local testing, code must be **ready-to-drop** (fully replaceable without manual editing).

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

## 9) Testing requirements

- **Before completing a task**, ensure all builds and tests succeed.
- Run:
  ```bash
  go test ./...
  ```
  All tests must pass.
- If logic was added or changed, provide new or updated **unit tests**.
- In the PR body, include test instructions and results summary.

---

## 10) Definition of Done (DoD)

A Codex task is considered **complete** only if all of the following hold:

1. **Build passes** successfully in the sandbox.
2. **All tests pass**, and new functionality is covered by tests.
3. The PR is **limited to scope** with no unrelated changes.
4. Code is **fully commented** and follows project conventions.
5. Logging is implemented and functional.
6. Documentation or rationale conflicts (if any) are flagged and approved.

---

## 11) Repository synchronization

- Operate on the **snapshot** of the assigned branch at task start.
- Before writing code, ensure the environment is synced to the latest `origin/<branch>` commit.
- Include the current **HEAD commit hash** in the PR description.
- If the branch changes upstream during task execution, **stop** and request a restart.

---

## 12) Documentation awareness

- Before editing or adding code, **read** the relevant documentation:
  - `docs/project_documentation.md`
  - `docs/ragionamenti.md`
- If proposed changes **conflict** with documentation, Codex must:
  1. Flag a **“Design Conflict”** in the PR.
  2. Request confirmation from the user before proceeding further.
- Never override documentation assumptions silently.

---

## 13) Rule violation & conflict reporting

- If Codex cannot satisfy all constraints due to limitations (e.g., missing stdlib feature, dependency need, conflicting rule), it must:
  1. Stop and describe the conflict in PR comments.
  2. Suggest possible resolutions or alternatives.

---

## 14) Atomicity & granularity of changes

- Large changes must be split into smaller, reviewable steps (separate PRs).
- Each PR should handle **one logical modification** only — e.g., “add logging”, “fix HTTP method validation”, “refactor config parser”.

---

## 15) Version control & commit conventions

- **PR title format:** `[Service|Module] — <short action>: <goal>`  
  Example: `aggregator — add slog and /health handler`
- **PR body must include:**
  - Diff Summary & Notes
  - Design Rationale
  - Test instructions
  - Impacts, risks, migration notes (if any)
- **Commit messages:** imperative style (e.g., “Add logging middleware”), one logical change per commit.

---

## 16) Limitations & escalation

- If a task cannot be completed within current Codex capabilities (e.g., missing environment tools, blocked dependency, ambiguous design), Codex must:
  - Halt the task,
  - State the limitation explicitly, and
  - Suggest how the user or GPT could revise the prompt.

---

## 17) Behavioral checklist

- Always operate on the latest repo snapshot.
- Ensure all builds and tests pass before PR completion.
- Always use `log/slog` for logging.
- Always follow Docker base images (`golang:1.23-alpine`, `alpine:3.20`).
- Comment and document every significant code section.
- Produce clear, human-readable PRs and commit messages.
- Never edit beyond scope or skip testing.
- Never use non-standard libraries without explicit permission.
- Never silently ignore contradictions with project documentation.

---

**End of AGENTS.md — Codex Operational Rules**
