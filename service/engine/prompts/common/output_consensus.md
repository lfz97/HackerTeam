# Output Consensus

**Operator and Worker** must adhere to this consensus. It defines the output format, file specifications, and raw data preservation requirements for task results.

---

## 1. File Output Specification

### 1.1 Report File

- **Format**: **Strictly Markdown (`.md`) only** for the report. JSON, TXT, and other formats are forbidden, except the machine-readable `TASK-<task_id>_result.json` verdict sidecar required by the Worker prompt.
- **Path**: `{{OUTPUTDIR}}/TASK-{task_id}_{YYYY-MM-DD-HH-MM}_{type}_result.md`, where `{YYYY-MM-DD-HH-MM}` is the current time (24-hour clock), and `{type}` is the task type (e.g., `recon`, `scan`, `exploit`, `postexploit`).
- **Content Requirements**: Professional, clear, authentic, and accurate. All data must come from actual execution. Do not fabricate or speculate.

### 1.2 Raw Output Directory

- **Each task MUST create a dedicated directory**: `{{OUTPUTDIR}}/TASK-{task_id}_{YYYY-MM-DD-HH-MM}_raw/`, where `{YYYY-MM-DD-HH-MM}` is the current time (24-hour clock).
- Execute `mkdir -p` to create this directory before beginning the task.
- **All complete raw output from external tools MUST be saved to this directory**.
  - Name files as `<tool_name>_<target_identifier>.<extension>`.
  - Saving only summaries or excerpts is forbidden.
  - If the tool supports `-o` / `--output` parameters, use them directly; otherwise use shell redirection `>`.

### 1.3 Reporting Timing

- Report task completion or conclusions in the conversation **only after** files have been fully written.
- Declaring task completion before files are written is strictly forbidden.

---

## 2. Required Report Sections

Every task's MD report MUST include at least the following sections (expand as needed):

1. **Task Overview** — Task objective, scope, execution time
2. **Command & Tool Log** — For each step: tool name, version, full command-line parameters, target, raw output file path, key output verbatim
3. **Raw Output Directory** — Full path `{{OUTPUTDIR}}/TASK-{task_id}_{YYYY-MM-DD-HH-MM}_raw/`, and a file listing of the directory contents
4. **Research Note** — Required when the brief's CLASS is research. One entry per claim:

| Field | Content |
|-------|---------|
| Claim | One sentence, falsifiable. |
| Evidence | Raw file path plus the verbatim line, request/response or command result that carries the claim. |
| Reproduction | The exact command, request or payload that reproduces it, and its preconditions. |
| Confidence | A percentage under the Vulnerability Consensus semantics, plus the observation that would move it. |
| Falsifier | The observation that would prove the claim wrong. |
| Sources | URL plus access date for every external claim; primary sources before summaries. |
| Next question | The most valuable question this opens, and the probe that discriminates it. |
| Negative results | What was ruled out, the evidence that ruled it out, and what evidence would reopen it. |

Raw tool output stays in the Command & Tool Log and the raw directory; the Research Note points at it and quotes only the carrying line. Coverage is a script, not an agent task: scanning output enters the note only after a probe has settled something about it.

**Negative results are first-class.** A ruled-out hypothesis is a result, not a failure. Record what was tested, what was observed, what it rules out, and what evidence would reopen it. "Tested, did not work" is not a negative result. An empty field must say why nothing was ruled out; silence reads as incomplete.

---

## 3. Conversation Reply JSON Specification

When a task is complete, output the following JSON summary in the conversation. The Operator or Worker prompt may add its own fields on top of the common fields.

### Common Fields (both roles MUST include)

```json
{
  "task_id": "<corresponding task ID>",
  "agent": "<Operator or Worker>",
  "status": "<status>",
  "report_path": "<full absolute path to the MD report file>",
  "raw_output_dir": "<full absolute path to the raw output directory>"
}
```

| Field | Description |
|-------|-------------|
| `task_id` | Task ID issued by Operator |
| `agent` | Role that produced the result: `Operator` or `Worker` |
| `status` | Task status; the role's own prompt defines the enum |
| `report_path` | Full absolute path to the MD report file |
| `raw_output_dir` | Full absolute path to the raw output directory |

### Role-Specific Fields

A role may add domain-specific fields on top of the common fields (e.g., `findings_summary`, `scan_summary`, `overall_risk`, etc.). See the Operator and Worker prompts for their own fields.

---

## 4. Vulnerability Structured Block

When a task discovers or confirms a vulnerability, the MD report **MUST** include a structured block for each vulnerability. This block is the **primary data source** for a PoC / script generation task producing Python reproduction scripts. Incomplete or vague blocks produce unusable scripts.

**Exemptions**: recon tasks are exempt — they do not produce vulnerabilities and use `priority` not `severity`.

### 4.1 Block Format

Each vulnerability structured block uses the following YAML format, enclosed in a fenced code block:

```yaml
# ── VULN-{id} ──
vuln_id: VULN-001
target:
  host: 192.168.1.100
  port: 443
  protocol: https
  base_url: "https://192.168.1.100:443"
type: sql_injection
severity: high
confidence: 90%

entry_point:
  method: GET
  path: "/api/login"
  headers:
    Cookie: "session=abc123"
    Content-Type: "application/x-www-form-urlencoded"
  params:
    id: "1"

payload:
  raw: "1' OR '1'='1' --"
  encoded: "1%27%20OR%20%271%27%3D%271%27%20--"
  injection_point: "params.id"

verification:
  method: "Compare response with and without payload"
  success_indicators:
    - "Response contains multiple user records"
    - "HTTP 200 + JSON array length > 1"
  failure_indicators:
    - "Response contains SQL syntax error"
    - "HTTP 500"

prerequisites:
  - "Login required to obtain session cookie"
  - "Target port 443 must be reachable"

evidence:
  raw_output_path: "/path/to/raw/sqlmap_target.txt"
  key_excerpt: |
    [14:23:01] [INFO] GET parameter 'id' is vulnerable...
```

### 4.2 Field Requirements per Task Type

Not every task type can fill every field. The table defines which fields are required, optional, or marked `pending_verification`:

| Field | Scan task | Verification task | Post-exploitation task |
|-------|---------|---------|-------------|
| `vuln_id` | Required (SCAN-xx) | Required (VULN-xx) | Required (VULN-xx) |
| `target` | Required | Required | Required |
| `type` | Required | Required | Required |
| `severity` | From tool label only | Required (Worker's proposal, Operator's ruling) | Required (Worker's proposal, Operator's ruling) |
| `confidence` | From tool label or estimated | Required (based on verification) | Required (based on verification) |
| `entry_point` | Required — extract from tool output | Required — must be from actual execution | Required — must be from actual execution |
| `payload` | Required — exact payload from tool output | Required — exact payload used | Required — exact command/technique used |
| `verification` | `pending_verification` | Required — must be specific, not vague | Required — must be specific |
| `prerequisites` | `pending_verification` | Required | Required |
| `evidence` | Required (raw output path) | Required (raw output path + key excerpt) | Required (raw output path + key excerpt) |

### 4.3 Anti-Patterns — What NOT to Write

The following are **forbidden** in structured blocks. Vague descriptions make the block useless for script generation:

| Anti-Pattern | Example (FORBIDDEN) | Correct Replacement |
|-------------|---------------------|---------------------|
| Vague payload | `payload: "SQL injection payload"` | `payload: { raw: "1' OR '1'='1' --", injection_point: "params.id" }` |
| Missing entry point | `entry_point: "login page"` | `entry_point: { method: "POST", path: "/api/login", params: { username: "admin", password: "PAYLOAD" } }` |
| Vague verification | `verification: "response changed"` | `verification: { success_indicators: ["HTTP 200 + JSON contains 'admin'"], failure_indicators: ["HTTP 302 redirect"] }` |
| No prerequisites | (field omitted entirely) | `prerequisites: ["Session cookie required", "Target must be reachable on port 8443"]` |
| Summary instead of evidence | `evidence: "successfully exploited"` | `evidence: { raw_output_path: "/path/to/raw.txt", key_excerpt: "uid=0(root) gid=0(root)" }` |

### 4.4 PostExploit Additional Fields

A post-exploitation task must also include these fields in its structured blocks, reflecting the post-exploitation context:

```yaml
session_info:
  type: "remote_shell"          # remote_shell | webshell | database_access | rdp_session | ssh_session
  access_level: "SYSTEM"         # Current privilege level
  session_id: "sess_03"         # Session identifier
  pivot_host: "10.0.0.5"        # Host being operated on (may differ from initial target)
lateral_technique: "pass-the-hash"  # Only for lateral movement findings
```

---

## 5. Purpose

*   **Consistency**: Every task output has the same structure, so the Operator can parse it uniformly.
*   **Traceability**: Trace every conclusion back to raw tool output.
*   **Reproducibility**: Third parties can independently verify the analysis process using raw output and command logs.
*   **No Omission**: Preserving raw output lets later tasks find information the summary report missed.
*   **Script-Ready**: Structured vulnerability blocks provide sufficient detail for automated PoC/exploit script generation.
