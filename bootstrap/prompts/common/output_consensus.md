# Output Consensus

**All Agents** must adhere to this consensus. It defines the output format, file specifications, and raw data preservation requirements for task results.

---

## 1. File Output Specification

### 1.1 Report File

- **Format**: **Strictly Markdown (`.md`) only**. JSON, TXT, or other formats are forbidden.
- **Path**: `{{OUTPUTDIR}}/TASK-{task_id}_{YYYY-MM-DD-HH-MM}_{type}_result.md`, where `{YYYY-MM-DD-HH-MM}` is the current time (24-hour clock), and `{type}` is defined by each Agent (e.g., `recon`, `scan`, `exploit`, `postexploit`).
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

Each Agent's MD report MUST include at least the following sections (each Agent may expand as needed):

1. **Task Overview** — Task objective, scope, execution time
2. **Command & Tool Log** — For each step: tool name, version, full command-line parameters, target, raw output file path, key output verbatim
3. **Raw Output Directory** — Full path `{{OUTPUTDIR}}/TASK-{task_id}_{YYYY-MM-DD-HH-MM}_raw/`, and a file listing of the directory contents

---

## 3. Conversation Reply JSON Specification

When a task is complete, output the following JSON summary in the conversation. Each Agent may append its own unique fields on top of the common fields.

### Common Fields (all Agents MUST include)

```json
{
  "task_id": "<corresponding task ID>",
  "agent": "<Agent name>",
  "status": "<status>",
  "report_path": "<full absolute path to the MD report file>",
  "raw_output_dir": "<full absolute path to the raw output directory>"
}
```

| Field | Description |
|-------|-------------|
| `task_id` | Task ID issued by Captain |
| `agent` | Current Agent name |
| `status` | Task status; each Agent defines its own status enum |
| `report_path` | Full absolute path to the MD report file |
| `raw_output_dir` | Full absolute path to the raw output directory |

### Agent-Specific Fields

Agents may append their own domain-specific fields on top of the common fields (e.g., `findings_summary`, `scan_summary`, `overall_risk`, etc.). See each Agent's prompt definition for details.

---

## 4. Purpose

*   **Consistency**: All Agent outputs have a consistent structure that Captain can uniformly parse.
*   **Traceability**: Trace back to raw tool output to verify Agent conclusions.
*   **Reproducibility**: Third parties can independently verify the analysis process using raw output and command logs.
*   **No Omission**: Preserving raw output gives subsequent Agents the opportunity to discover information missed in the summary report.
