# Atlas OpenCode Entrypoint — {{PROJECT_NAME}}

- This projection is `.opencode/atlas.md`.
- Root `AGENTS.md` is the project authority; do not bypass it.
- For any request that may mutate product files, route through `.opencode/agents/atlas-orchestrator.md` first: classify → resolve governance → gate → then act.
- Do not implement material product changes directly because planning failed or the user did not name SDD/OpenSpec.
- Agents: `.atlas/agent-registry.md` under `.opencode/agents/`.
- SDD contract when present: `.atlas/contracts/sdd-openspec.md`.
- Skills: `.atlas/skill-registry.md` (Home-canonical pins only).
- Delivery: OpenCode adapter ≠ hosting provider; remote name `origin` ≠ Cursor Origin. Honor `.atlas/config.yaml` `source_control.*`.
- This file is an entrypoint only, not a second policy source.
