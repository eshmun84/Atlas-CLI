## OpenCode Adapter Guidance

OpenCode must treat root `AGENTS.md` as the project authority for this repository.

`.opencode/atlas.md` is only the OpenCode-native entrypoint. It exists to point OpenCode at this contract, at `.opencode/agents/atlas-orchestrator.md`, at `.atlas/agent-registry.md`, and at `.atlas/contracts/sdd-openspec.md`. It is not an independent policy source and must not duplicate or dilute the full Atlas contract.

Atlas-owned OpenCode agents are materialized under `.opencode/agents/` and cataloged in `.atlas/agent-registry.md`. Prefer `atlas-orchestrator` for routing. For SDD/OpenSpec phase work, follow `.atlas/contracts/sdd-openspec.md`. Developer-owned non-Atlas agents in that directory remain outside Atlas ownership.

OpenCode agents, workers, reviewers, commands, and plugins are execution surfaces, not independent authorities. Delegated work produces evidence; it does not grant delivery approval.

OpenCode must not bypass Atlas delivery or Git rules. Explicit human request is still required for commit, push, publish, and other irreversible delivery actions. If OpenCode guidance conflicts with this file, this file wins.
