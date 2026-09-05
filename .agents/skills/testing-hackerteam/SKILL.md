---
name: hackerteam-tui-smoke-testing
description: Build and visually smoke-test HackerTeam startup without model credentials, and distinguish startup evidence from model-routing coverage.
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

## Devin Secrets Needed

Credential-free startup checks: none.

Model-dependent testing requires a provider API key supplied through the approved secret mechanism and configured as `model.apikey` in `.HackerTeam/HackerTeam.yaml`, plus matching `model.baseurl`, `model.model`, and `model.apitype`. There is no fixed repository-defined environment-secret name; do not invent one or expose values in recordings.
