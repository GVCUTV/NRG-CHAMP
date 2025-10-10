# AGENTS.md — Project Instruction Rules for NRG CHAMP

> **Apply these rules in every task unless explicitly overridden.**

---

## 1. General Compliance & Change Control

- Always obey **all** rules below — do not violate unless the user *explicitly instructs* you to break them.  
- **Never** change what the user didn’t explicitly ask you to change — unless you first ask for permission.  
- If you must break a rule (for a justified reason), you must **explicitly report** it at the start of your output, *after the diff section*, explaining:
  1. Which rule was broken  
  2. How it was broken  
  3. Why the break was necessary  

---

## 2. Diff & Rule Violation Reporting

- Every output must begin with a **clear diff summary**, showing all added, removed, or modified lines/files.  
- Immediately after the diff, if any rule was broken, include a **“Rule Violation Report”** section.  
- Only after those must you provide the actual code or content output.

---

## 3. Output Format & File Generation

- When generating code outputs, you must provide **downloadable files**.  
- If multiple files are produced, package them into a **single zip archive**.  
- You must **never** output the code in a non-downloadable (inline-only) form when full files or archives are expected.

---

## 4. Full File, Ready-to-Drop

- Always generate **complete files**, not just snippets, unless explicitly told you can.  
- The output files must be **ready-to-drop**: I should be able to replace existing files without needing manual edits.  
- You must **preserve any code you did not intend to modify** (imports, comments, structure) exactly as is.

---

## 5. Versioning & File Header

- Each file must begin with a version comment on the **first line**, e.g.:
  ```go
  // v3
  // myservice.go
  ```  
- That version must be **incremented** from the previous version (v2 → v3, etc.).  
- If no previous version is known, start from **v0**.  
- The second line must include the file name, as shown above.

---

## 6. Commenting & Design Rationale

- All code must be **fully commented** with natural, human-style explanations (no AI disclaimers).  
- If you make design choices, short rationale or justification follows in comments or a small note after the code.

---

## 7. Logging Policy

- Every operation executed by the code must be logged to **both**:
  1. Standard output (console)  
  2. A logfile  
- **Always** use Go’s **`slog`** (or `log/slog`) for all logging — no alternative logging libraries.

---

## 8. Package / Dependency Policy

- Use only **Go standard libraries** unless explicitly instructed otherwise.  
- If you determine a needed library is not in the standard library, you must **stop** and ask me how to proceed (which library to use or what alternative).

---

## 9. Docker / Build / Runtime Rules

- For any Dockerfile:
  - **Build stage** must use `golang:1.23-alpine`.  
  - **Runtime stage** must use `alpine:3.20`.  
  - If a health check endpoint is required, it should use `/health` (GET) to confirm service status.

---

## 10. HTTP Method Discipline

- In server/handler code, each endpoint must **only accept the correct HTTP method**:
  - `GET` for reads/status  
  - `POST` for commands or updates  
  - `PUT` / `PATCH` for config changes  
  - `DELETE` for removals  
- Reject or error on other methods.

---

## 11. Handling Limitations & Uncertainty

- If you believe a part of my prompt is **unrealizable** due to your current GPT version or system constraints, **stop execution** and explicitly **notify me**:
  - Which part is problematic  
  - Why it’s not feasible  
  - Propose alternative prompt or adjustment

---

## 12. Task Instructions & Scope in Prompts

- Prompts should always specify:
  1. **What** needs to change (goal)  
  2. **Which files or modules** are affected  
  3. What must **not be changed**  
- If needed, the prompt can specify the **last known version number** for continuity.

---

## 13. Echo / Confirmation

- Before you produce final code, you may **echo back your understanding of the rules** or specific constraints for that task so I confirm correctness.  
- If your echo is wrong or incomplete, I can correct it before you generate full output.

---

## 14. Incremental / Subtask Approach

- For large or complex changes, break them into **smaller subtasks** (e.g. “first add logging,” “then modify endpoint logic”) so rule compliance is easier to enforce and review.

---

**End of AGENTS.md**
