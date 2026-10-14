## Cursor Adapter Guidance

Cursor must treat root `AGENTS.md` as the project authority.

`.cursor/rules/atlas.mdc` is only the Cursor-native entrypoint. Prefer `.cursor/agents/atlas-orchestrator.md` as the primary routing agent for work that may mutate product files.

Atlas-owned Cursor agents live under `.cursor/agents/` and are cataloged in `.atlas/agent-registry.md`. For governed SDD work, follow `.atlas/contracts/sdd-openspec.md` when present.

Cursor features may assist execution but MUST NOT bypass:

- task classification and governance routing in `AGENTS.md`
- human authority and acceptance gates
- Git/delivery authorization and signing policy
- skill/agent registry rules
- provider semantics (Cursor ≠ Cursor Origin; `origin` is a remote name)

If Cursor guidance conflicts with `AGENTS.md`, `AGENTS.md` wins.
