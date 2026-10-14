## OpenCode Adapter Guidance

OpenCode must treat root `AGENTS.md` as the project authority.

`.opencode/atlas.md` is only the OpenCode-native entrypoint. Prefer `.opencode/agents/atlas-orchestrator.md` as the primary routing agent for work that may mutate product files.

Atlas-owned OpenCode agents live under `.opencode/agents/` and are cataloged in `.atlas/agent-registry.md`. For governed SDD work, follow `.atlas/contracts/sdd-openspec.md` when present.

OpenCode features may assist execution but MUST NOT bypass:

- task classification and governance routing in `AGENTS.md`
- human authority and acceptance gates
- Git/delivery authorization and signing policy
- skill/agent registry rules
- provider semantics (OpenCode ≠ hosting provider; `origin` is a remote name)

If OpenCode guidance conflicts with `AGENTS.md`, `AGENTS.md` wins.
