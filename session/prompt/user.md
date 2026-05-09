Conversation to summarize:

{conversation_text}

---

Produce the `<analysis>` and `<summary>` blocks now, following the system instructions and the template below.

Section guidance (apply to the `<summary>` block):

1. **Primary Request and Intent** — 1–3 sentences. The user's overall goal across the conversation, plus any explicit constraints (deadlines, must-use libs, forbidden actions).
2. **Key Technical Concepts** — bullet list of domain/tech concepts actually relevant to the work (frameworks, protocols, algorithms, repo-specific terms). Skip generic/common knowledge.
3. **Files and Code Sections** — for each file *touched, read, or referenced as load-bearing*: `path/to/file` → 1 line on why it matters → minimal code excerpt (≤ ~20 lines, elide with `// ...` where safe). Prefer signatures and changed regions over full bodies. Omit files that were only glanced at.
4. **Errors and Fixes** — bullet list of `error → root cause → fix applied (or attempted)`. Include user feedback that corrected the agent's course.
5. **Problem Solving** — non-trivial reasoning, design choices, or trade-offs made. Do not repeat items already covered in §4 or §8.
6. **User Messages (condensed)** — chronological, deduplicated list of the user's intents/asks (paraphrase, do not transcribe). Merge repeated asks. This is for tracking how requirements evolved.
7. **Pending Tasks** — only tasks the user explicitly asked for that are NOT yet done. Empty list is fine.
8. **Current Work** — what was being done immediately before this summary point: file/function, last action, last result.
9. **Optional Next Step** — the single most natural next action, tightly aligned with §8 and §7. Quote the user's own words (with `> "..."`) to justify it. If unclear or the task looks complete, write `None`.

Format skeleton (fill in each section; keep within the global word budget):

<analysis>
[brief notes on what you kept/dropped and why]
</analysis>

<summary>
1. Primary Request and Intent:
[...]

2. Key Technical Concepts:
- [...]

3. Files and Code Sections:
- `path/to/file`
  - why it matters: [...]
  - excerpt:
    ```
    [minimal snippet]
    ```

4. Errors and Fixes:
- [error] → [cause] → [fix]

5. Problem Solving:
[...]

6. User Messages (condensed):
- [...]

7. Pending Tasks:
- [...]

8. Current Work:
[...]

9. Optional Next Step:
[...]
</summary>
