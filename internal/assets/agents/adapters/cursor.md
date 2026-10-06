## Cursor Adapter Guidance

Cursor must treat root `AGENTS.md` as the project authority for this repository.

`.cursor/rules/atlas.mdc` is only the Cursor-native entrypoint. It exists to point Cursor at this contract and at `.cursor/agents/atlas-orchestrator.md`. It is not an independent policy source and must not duplicate or dilute the full Atlas contract.

Atlas-owned Cursor agents are materialized under `.cursor/agents/` and cataloged in `.atlas/agent-registry.md`. Prefer `atlas-orchestrator` for routing. Developer-owned non-Atlas agents in that directory remain outside Atlas ownership.

Cursor features such as rules, Composer, agent modes, and any subagent-style helpers remain inside Atlas authority. They may assist execution, but they must not bypass:

- human authority and explicit approval boundaries
- scope control
- Git and delivery authorization
- verification honesty
- skill and contract loading rules

If Cursor guidance conflicts with this file, this file wins.
