---
name: hackerteam-tui-smoke-testing
description: Test HackerTeam startup and bounded offline artifact routing through the real TUI, with explicit credential and runtime-evidence checks.
---

# HackerTeam TUI smoke testing

- Build natively from the repository root with `go build -ldflags "-s -w" -o HackerTeam ./cmd`. CGO and a compatible Go toolchain are required; consult the environment blueprint.
- Launch the built executable in a maximized GUI terminal when recording. Do not run a TUI through a noninteractive shell pipe.
- Runtime state lives in `.HackerTeam/` beside the executable, not beside the shell's working directory. Avoid `go run` for persistent test configuration.
- On a fresh installation, startup creates `HackerTeam.yaml`, displays a notice asking for configuration changes, and waits for a keypress to exit. Preserve existing user configuration rather than deleting it to recreate this condition.
- Restarting with the default placeholder configuration can reach the new-conversation input without submitting a model request. Ctrl+K opens/closes help; `/exit` followed by Enter exits.
- Label these checks as regression/startup coverage. They cannot verify Captain scheduling, agent dispatch, mode prerequisites, or task completion.
- Never submit tasks with placeholder credentials merely to demonstrate an expected authentication failure. With no configured model, report routing coverage as blocked.
- For benign routing tests with a configured model, use narrowly scoped offline file-processing tasks and independently verify their outputs. Do not use real targets or infer skipped stages from absence of activity before a task is submitted.
- A successful tiny preflight may end with `finish_reason=length` and an empty final message when a reasoning model spends the token budget on reasoning. Distinguish this from authentication/schema errors; normal application token limits are needed to test real tool use.
- To exercise natural fallback routing, request a multi-step local Python standard-library task with an explicit input/output contract without naming the agent to select. Base64 decoding plus saved-JSON hash verification is a harmless example. Verify the result independently rather than trusting the agent's summary.
- Capture the real terminal with `script` output logging. `.HackerTeam/HackerTeam.log` includes `Executing tool ... with args:` records suitable for checking actual dispatches and every `todo_write` snapshot. Check that each snapshot has at most one `in_progress` item and distinguish these log assertions from visible UI assertions.
- Tool argument/result text may have poor contrast in some terminals. Preserve screenshots of the issue and use the runtime log to corroborate exact states; do not claim hidden details were visible.
- Optional `strace -f -e trace=connect,execve` around the app records connection/command metadata without request headers. Compare destinations with the model-provider DNS result and inspect executed commands; qualify this as observed-process evidence, not a universal network guarantee.
- Keep temporary model config private (mode 0600), avoid displaying it, and restore the original after testing. Scan shared logs/captures for the exact key before attaching them. Never store a key in an on-disk test script or command argument.

## Devin Secrets Needed

Credential-free startup checks: none.

Model-dependent testing requires a provider API key supplied through the approved secret mechanism and configured as `model.apikey` in `.HackerTeam/HackerTeam.yaml`, plus matching `model.baseurl`, `model.model`, and `model.apitype`. The temporary session secret used for this test was `HACKERTEAM_MODEL_API_KEY`; confirm the available secret reference each session rather than assuming it persists.
