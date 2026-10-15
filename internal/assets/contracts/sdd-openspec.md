# Atlas SDD / OpenSpec Operational Contract

Atlas-owned operational contract for Spec-Driven Development with **real OpenSpec**.

Canonical project path: `.atlas/contracts/sdd-openspec.md`  
Canonical Home path: `$ATLAS_HOME/assets/contracts/sdd-openspec.md`

**Activation:** Materialized only when `.atlas/config.yaml` has `governance.workflow=sdd` **and** `governance.spec_engine=openspec`.

When activated, material product work MUST enter this governed route automatically. The human need not say “plan”, “spec”, “SDD”, or “OpenSpec”.

## 0. Real OpenSpec provider (required)

OpenSpec is a **real Spec Engine**, not an Atlas-emulated layout.

Required for governed SDD:

1. OpenSpec CLI (`openspec`) installed and on PATH
2. CLI version inside Atlas support (`>= 1.14.0`, major `< 2`) **and** equal to `governance.openspec_version` (exact project pin)
3. OpenSpec project initialized by **`openspec init`** (not by mkdir)
4. Required capabilities (`version/list/status/instructions/validate` JSON, `init --tools`, `update`, `archive`)

If any requirement fails: governed SDD is **BLOCKED**. Status/Doctor remain read-only. NEVER invent CLI output. NEVER silently fall back to filesystem-only emulation. NEVER automate Git/delivery remotes/push/PR/merge/tag.

### Ownership

| Owner | Surfaces |
|-------|----------|
| **Atlas** | Task classification, routing, human gates, testing/review, Memory/Context, fail-closed policy, this contract |
| **OpenSpec CLI** | `openspec init/update/new/status/instructions/validate/archive`, `openspec/` tree, `config.yaml`, generated opsx skills/commands |
| **Agent/human** | Markdown artifacts (proposal/specs/design/tasks) and product code, guided by OpenSpec instructions |

### Agent workflow entry points

Use OpenSpec-projected workflows (do not invent parallel headers/templates):

- Cursor/OpenCode commands: `/opsx-propose`, `/opsx-apply`, `/opsx-update`, `/opsx-archive`, `/opsx-explore`, `/opsx-sync` (tool-specific spelling)
- Matching skills under `.cursor/skills/openspec-*` / `.opencode/skills/openspec-*`

Atlas SDD agents govern gates; they **delegate** artifact lifecycle to OpenSpec (`openspec new change`, `openspec status/instructions --json`, author files, `openspec validate`, `openspec archive`).

## 1. Purpose

Provide a phase-aware procedure so Atlas agents:

- classify/route material work into governed SDD when configured
- keep scope/authority with the human
- produce evidence instead of invented progress
- use real OpenSpec for change scaffolding, instructions, validation, and archive

`AGENTS.md` remains project authority. If guidance conflicts, `AGENTS.md` wins.

### 1.1 Fail-closed activation

If this contract is required but missing/corrupt, provider is not ready, pin mismatches, or required artifacts/acceptance cannot be established:

- status = `blocked` or `interaction_required`
- MUST NOT mutate product files
- MUST NOT fall back to direct implementation because “planning failed”

## 2. Authority model

| Surface | Role |
|---------|------|
| Human | Final authority for scope, acceptance, delivery, merge, push, PR, release |
| `AGENTS.md` | Project runtime authority |
| This contract | Operational SDD/OpenSpec procedure |
| Registries/manifest | Atlas agent/skill/runtime catalogs |
| `atlas-orchestrator` | Primary router / ask / propose / stop conductor |
| SDD phase agents | One phase under this contract; OpenSpec for provider lifecycle |
| Review agents | Evidence only; never final authority |
| `atlas-worker` | Bounded delegated execution; no scope expansion |

## 3. Phases

1. Init → 2. Explore → 3. Research? → 4. Propose → 5. Update? → 6. Implement → 7. Verify → 8. Archive

Propose → Implement requires human acceptance of planning artifacts.

Before Implement, OpenSpec change artifacts required by the active schema must exist and be sufficient (default spec-driven: proposal + tasks; specs/design when the schema/status requires them). Use `openspec status --json` / `openspec instructions --json` — do not invent paths or headers.

## 4. Cross-cutting rules

- Prefer OpenSpec instructions over inventing delta headers or archive naming
- `openspec validate` / `openspec archive` own structural validation and archive merge
- Atlas Memory is distilled durable knowledge; OpenSpec is structured change/spec history — do not duplicate OpenSpec bodies into Memory
- Never create remotes, push, open PRs, merge, tag, or publish releases as Atlas Core
- Skills: load only enabled exact pins; OpenSpec `openspec-*` skills are provider-owned

## 5. Stop / ask

Stop and ask when: provider not ready, pin mismatch, schema instructions unclear, acceptance missing, evidence insufficient, or scope would expand.
