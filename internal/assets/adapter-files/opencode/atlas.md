# Atlas OpenCode Entrypoint

Project: {{PROJECT_NAME}}

- Root `AGENTS.md` is the project authority; do not bypass it.
- Prefer the Atlas orchestrator at `.opencode/agents/atlas-orchestrator.md` for routing and governed work.
- Atlas agents are listed in `.atlas/agent-registry.md` and materialized under `.opencode/agents/`.
- This file (`.opencode/atlas.md`) is an OpenCode-native entrypoint only, not a full Atlas contract.
- OpenCode agents/workers/reviewers/commands/plugins are execution surfaces, not independent authorities.
- Follow Atlas delivery and Git authorization rules from `AGENTS.md`.
- Project configuration lives under `.atlas/config.yaml`.
