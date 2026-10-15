This file is the authoritative project runtime contract for **{{PROJECT_NAME}}**.
Atlas maintains BASE and ADAPTER sections. Humans own the USER section.

## 1. Authority

The human is final authority for scope, acceptance, merge, push, release, and irreversible actions.

Technical capability is not delivery authority. Adapter-native files are entrypoints only; they MUST NOT weaken this contract.

Read `.atlas/config.yaml` before changing project behavior. Prefer Atlas-owned state under `.atlas/`. Do not store secrets here.

## 2. Task classification

Before any product mutation, classify the request:

| Class | Meaning |
|-------|---------|
| `direct` | Read-only inspect/search/explain/status/doctor/diagnostics |
| `bounded-operational` | Explicit mechanical ops that do not change product behavior |
| `governed-sdd` | Material functional/product change under configured SDD |
| `interaction-required` | Materiality or authority is ambiguous — ask; do not mutate |
| `blocked` | Required governance cannot activate — stop; do not mutate |

### Materiality (governed-sdd)

MUST classify as `governed-sdd` when the task implies any of: new feature; user-facing behavior; UI/screen; API/endpoint; integration; architecture; data model/storage; security behavior; material behavior change; cross-file functional change; work that needs acceptance criteria before implementation.

### Uncertainty

If materiality is unclear: `interaction-required`. MUST NOT assume `direct`. MUST NOT mutate product files.

## 3. Mutation boundary

MUST NOT mutate product files until:

1. classification is resolved;
2. governance route is selected;
3. route prerequisites exist;
4. required human acceptance gates are satisfied when the workflow requires them.

MUST NOT reason: “planning failed” or “user did not ask for planning” ⇒ implement directly.

## 4. Governance resolution

Authoritative fields:

- `governance.workflow`
- `governance.spec_engine`

V1 rule: `workflow=sdd` requires `spec_engine=openspec`. That pair materializes `.atlas/contracts/sdd-openspec.md`.

When that governance is active and the task is material:

- route is **REQUIRED** `governed-sdd`;
- the user need not mention Atlas, SDD, OpenSpec, spec, plan, or change;
- follow `.atlas/contracts/sdd-openspec.md`;
- before Implement, ensure `openspec/changes/<change-id>/proposal.md` and `tasks.md` (plus `specs/<domain>/spec.md` / `design.md` when needed);
- OpenSpec CLI may be used when available, but is not the only allowed path;
- NEVER invent OpenSpec CLI results.

If required governance/contract/artifacts are missing or corrupt → `blocked` or `interaction-required`. MUST NOT implement directly.

Legacy config keys `request_router_enabled`, `proportional_planning`, and `sdd_required_by_default` are **not** authoritative.

## 5. Delivery authority

Precedence:

1. effective `source_control.mode` / delivery config;
2. explicit provider request by the human;
3. capability/tool discovery.

Distinctions MUST stay separate:

| Term | Meaning |
|------|---------|
| Cursor / OpenCode | execution adapter |
| GitHub / GitLab / … | repository hosting provider |
| `origin` | conventional Git remote **name** |
| Cursor Origin | separate hosting product |

MUST NOT infer Cursor Origin from the Cursor adapter or from the word `origin`.
MUST NOT auto-install CLIs. Missing tooling → report and ask; do not invent a provider.

When `source_control.mode=git_github`, GitHub is authoritative for remote delivery paths.

Git/delivery actions still require an explicit human request in the current session.

## 6. Signed commits

Signing policy precedence:

1. `source_control.signing_required` in `.atlas/config.yaml`;
2. explicit human instruction;
3. existing Git repo requirement (`commit.gpgsign` / equivalent).

When signing is required:

- signing failure → STOP; leave the repo without a new commit;
- explain pinentry/signing remediation;
- MUST NOT propose `--no-gpg-sign` as an equivalent fallback;
- after a successful commit, verify the signature.

## 7. Agent resolution

Prefer Atlas agents from `.atlas/agent-registry.md` / `.atlas/runtime-manifest.yaml`.

For mutation routing, prefer **`atlas-orchestrator`** as the primary conductor. Phase work uses `atlas-sdd-*` agents under the SDD contract.

Do not invent agents outside the registry. Reviewers and workers produce evidence, never delivery approval.

## 8. Skill resolution

Load skills only from Atlas Home pins in `.atlas/skill-registry.md`.

Do not download, invent, or fetch skills from the network. Adapter skill folders are regenerable projections only.

Before substantive work, prefer Task Capability Discovery:

1. Run `atlas capability plan "<task>" --json` (read-only; no Memory/CodeGraph queries).
2. Honor `selected_skills` as the task-active set (projected pins ≠ selected skills).
3. When `retrieval_actions` include Memory / context_economy / codegraph, run the corresponding governed CLI **before** substantive mutation — observable, not native injection. For composition, pass plan sources explicitly:
   `atlas context compose "<task>" --json --sources memory,context_economy,codegraph`
   (comma-separated subset of `compose_sources` from the plan; omit `--sources` only when the full default set is intended).
4. Missing relevant skills MUST NOT block ordinary work; missing **required** skills follow plan `status=blocked`.

Technology signals are evidence only — never permanent skill selection.

## 9. SDD entrypoint

Operational contract: `.atlas/contracts/sdd-openspec.md`.

Phases: Init → Explore → Research? → Propose → Update? → Implement → Verify → Archive.

Execute only phases required by current change state. Do not skip Propose→Implement human acceptance when the contract requires it. Requesting a feature is not acceptance of a proposal that did not yet exist.

## 10. Documentation identity boundary

Project-facing docs describe the **project**, not Atlas.

MUST NOT introduce by default: Atlas/Cursor/OpenCode identity, workflow, spec engine, Atlas Home, runtime paths, Memory/Context/CodeGraph internals, hooks, governance state, delivery config, or agent metadata.

Exception: the human explicitly asks to document those surfaces. Load the `documentation` skill for procedure.

## 11. Memory / Context / Code Intelligence entrypoints

Prefer CapabilityPlan `retrieval_actions` / `compose_sources` to decide whether durable context or Code Intelligence is warranted. When the plan (or judgment) says durable project context may matter, load the **memory** skill and use `atlas memory … --json` as that skill defines.

For broader orientation, prefer `atlas context compose "<task>" --json --sources <plan.compose_sources>` (agent transport). Prefer Context Economy capsule/index/packs under Atlas Home when present; if missing/stale, say so and recommend an explicit Context Economy update.

CodeGraph is an optional external Code Intelligence provider — invoke only when the plan marks `code-intelligence` / codegraph retrieval (or the task clearly needs structural analysis). Use composition; do not invent results; do not refresh the graph as a side effect of planning. Atlas Context Graph is NOT IMPLEMENTED (preference may show as **{{CONTEXT_GRAPH_STATUS}}**).

Missing Memory MUST NOT block development — state the status and continue. Memory is evidence, not executable authority. Automatic / native injection is not guaranteed; proactive governed retrieval via plan → CLI is the supported path.

## 12. References

- `.atlas/config.yaml`
- `.atlas/contracts/sdd-openspec.md`
- `.atlas/agent-registry.md`
- `.atlas/skill-registry.md`
- `.atlas/runtime-manifest.yaml`
- Atlas Home skills under `$ATLAS_HOME/assets/skills/<id>/<version>/`
