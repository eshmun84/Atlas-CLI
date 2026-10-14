# Atlas SDD / OpenSpec Operational Contract

This file is the Atlas-owned operational contract for Spec-Driven Development (SDD) with OpenSpec-compatible change artifacts.

Canonical project path: `.atlas/contracts/sdd-openspec.md`  
Canonical Home path: `$ATLAS_HOME/assets/contracts/sdd-openspec.md` (mirrored from bundled assets)

**Activation:** This contract is materialized only when `.atlas/config.yaml` has `governance.workflow=sdd` **and** `governance.spec_engine=openspec`.

When activated, material product work MUST enter this governed route automatically. The human need not say “plan”, “spec”, “SDD”, or “OpenSpec”.

This contract does **not** require OpenSpec CLI for every step. Agents MUST produce/use live OpenSpec-compatible change artifacts before Implement. CLI may be used when available and appropriate. NEVER invent CLI output. NEVER automate Git/delivery.

### Change artifact layout (Atlas-recognized)

Atlas recognizes the following OpenSpec change tree (same paths the OpenSpec adapter inspects). Do not invent a parallel layout.

Active change root:

```text
openspec/changes/<change-id>/
```

Minimum material artifacts **before Implement**:

| Path | Role |
|------|------|
| `openspec/changes/<change-id>/proposal.md` | Required. Intent/problem, scope, non-scope when applicable, acceptance criteria / requirements, approach when needed |
| `openspec/changes/<change-id>/tasks.md` | Required. Implementable, verifiable tasks |
| `openspec/changes/<change-id>/specs/<domain>/spec.md` | When the change needs domain requirements/specs — one or more domains as needed |
| `openspec/changes/<change-id>/design.md` | Optional. Include when the change needs an explicit design commitment |

Do not require unused files for simple changes. Archived changes live under `openspec/changes/archive/` (observation only for Archive phase).

## 1. Purpose

Provide a single, phase-aware operating procedure so Atlas agents:

- classify and route material work into governed SDD automatically when configured
- frame, explore, research, propose, update, implement, verify, and archive changes consistently
- keep scope and authority with the human
- produce evidence instead of invented progress
- remain compatible with OpenSpec change layouts without pretending CLI results exist

`AGENTS.md` remains the project authority. This contract specializes SDD/OpenSpec behavior under that authority. If guidance conflicts, `AGENTS.md` wins.

### 1.1 Fail-closed activation

If this contract is required for the route but missing/corrupt, or required change artifacts/acceptance cannot be established:

- status = `blocked` or `interaction_required`
- MUST NOT mutate product files
- MUST NOT fall back to direct implementation because “planning failed” or “user did not request planning”

## 2. Authority model

| Surface | Role |
|---------|------|
| Human | Final authority for scope, acceptance, delivery, merge, push, PR, release |
| `AGENTS.md` | Project runtime authority |
| This contract | Operational SDD/OpenSpec procedure |
| `.atlas/agent-registry.md` | Catalog of Atlas agents and project paths |
| `.atlas/skill-registry.md` | Enabled exact skill pins index (Home-canonical) |
| `.atlas/runtime-manifest.yaml` | Machine-readable runtime surfaces |
| `atlas-orchestrator` | Primary router / ask / propose / stop conductor |
| SDD phase agents | Execute one phase under this contract |
| Review agents | Adversarial/review evidence only; never final authority |
| `atlas-worker` | Bounded delegated execution; no scope expansion |

Delegated output is evidence, not approval.

## 3. Phases and transitions

Ordered phases:

1. **Init** → 2. **Explore** → 3. **Research** (optional when alternatives are clear) → 4. **Propose** → 5. **Update** (as needed after feedback) → 6. **Implement** → 7. **Verify** → 8. **Archive**

Allowed transitions:

| From | To | Gate |
|------|----|------|
| Init | Explore | Intent framed; open questions listed or answered |
| Explore | Research | Findings need comparative investigation |
| Explore | Propose | Enough context to draft a concrete proposal |
| Research | Propose | Alternatives compared; recommendation stated as recommendation |
| Propose | Update | Human requested changes to the proposal |
| Propose | Implement | Human explicitly accepted the proposal/scope |
| Update | Propose | Revised proposal ready for re-acceptance |
| Update | Implement | Human explicitly accepted the revised scope |
| Implement | Verify | Implementation claimed complete within accepted scope |
| Implement | Update / Propose | Material conflict discovered; stop and re-scope with human |
| Verify | Archive | Verification reported with evidence (pass/fail/not-run) |
| Verify | Implement | Explicit human delegation to fix failures within accepted scope |
| Any | Stop | Missing authority, conflict with `AGENTS.md`, or invented evidence would be required |

Do not skip Propose → Implement acceptance. Do not jump to Archive without Verify evidence or an explicit human waiver recorded as a limitation.

## 4. Phase contracts

### 4.1 Init (`atlas-sdd-init`)

**Purpose:** Validate governance context and frame change intent without inventing project goals.

**Expected input:** Human request; `AGENTS.md`; `.atlas/config.yaml`; registry/manifest when present; optional existing SDD/OpenSpec artifacts.

**Expected output:** Init brief with intent, in/out of scope candidates, constraints, assumptions, open questions, and recommended next phase.

**Allowed:** Read project/runtime surfaces; ask clarifying questions; propose artifact shape for later phases.

**Forbidden:** Invent project purpose; implement code; declare stakeholder approval; rewrite `AGENTS.md` policy; execute OpenSpec CLI; create silent Git history.

**Ask when:** Base intent, success criteria, or non-negotiable constraints are missing.

**Stop when:** Project is not Atlas-governed and authority is unclear; required runtime surfaces are missing/drifted and repair is needed; continuing would invent goals.

**Human approval:** Not required to ask questions. Required before treating inventively guessed goals as fact.

**Minimum evidence:** Named intent source (human quote or explicit artifact); governance/runtime observations; open-question list.

### 4.2 Explore (`atlas-sdd-explore`)

**Purpose:** Read-only context mapping before solution design.

**Expected input:** Framed intent from Init; relevant paths; constraints.

**Expected output:** Findings, constraints, risks, options, open questions. No final design commitment.

**Allowed:** Read code/docs/config; summarize structure; list options and risks.

**Forbidden:** Edit code; design the final solution as accepted; expand scope; invent repo facts; execute OpenSpec CLI; Git automation.

**Ask when:** Access/context is insufficient to avoid speculation on a material point.

**Stop when:** Exploration is sufficient for Research or Propose, or further claims would be speculative.

**Human approval:** Not required for read-only exploration.

**Minimum evidence:** Paths inspected; concrete findings; explicit unknowns.

### 4.3 Research (`atlas-sdd-research`)

**Purpose:** Compare alternatives with separated evidence, inference, and recommendation.

**Expected input:** Explore findings; decision questions; constraints.

**Expected output:** Alternatives matrix; assumptions; evidence vs inference vs recommendation; risks of each option.

**Allowed:** Read-only investigation; document tradeoffs; recommend with rationale labeled as recommendation.

**Forbidden:** Implement; treat recommendation as approval; hide assumptions; invent benchmark/command results; OpenSpec CLI; Git automation.

**Ask when:** A decisive constraint is unknown and changes the ranking.

**Stop when:** Comparison is sufficient for Propose, or blocked on missing facts.

**Human approval:** Not required to recommend; required to accept a recommendation as project commitment.

**Minimum evidence:** At least two considered options (or explicit reason only one exists); labeled evidence/inference/recommendation.

### 4.4 Propose (`atlas-sdd-propose`)

**Purpose:** Turn intent + explore/research into a concrete, reviewable change proposal.

**Expected input:** Accepted intent; explore/research outputs; constraints.

**Expected output:** Proposal with scope, non-scope, tasks, risks, validation plan, rollback/limits, and explicit acceptance request; files under `openspec/changes/<change-id>/` (`proposal.md`, `tasks.md`, and specs/design when needed).

**Allowed:** Create/update that change tree; define tasks and checks; request human acceptance.

**Forbidden:** Implement before acceptance; silently widen scope; claim approval; invent OpenSpec CLI output; Git delivery.

**Ask when:** Scope boundaries or acceptance criteria are ambiguous.

**Stop when:** Proposal cannot be made concrete without inventing requirements.

**Human approval:** **Required** before Implement. Proposal ≠ acceptance.

**Minimum evidence:** Scope/non-scope; task list; validation plan; risk list; acceptance question.

### 4.5 Update (`atlas-sdd-update`)

**Purpose:** Revise an existing proposal/change contract when new information arrives, preserving limits.

**Expected input:** Prior proposal; human feedback; new findings.

**Expected output:** Updated proposal with change log of what shifted, what stayed, and what still needs acceptance.

**Allowed:** Edit the same `openspec/changes/<change-id>/` tree under Atlas/human direction; restate limits.

**Forbidden:** Implement by itself; silently reopen closed scope; drop acceptance gates; invent OpenSpec CLI output; Git automation.

**Ask when:** Feedback conflicts with prior acceptance or with `AGENTS.md`.

**Stop when:** Update would require inventing stakeholder intent.

**Human approval:** Required again before Implement if scope/validation materially changed.

**Minimum evidence:** Diff of proposal commitments; residual open questions.

### 4.6 Implement (`atlas-sdd-implement`)

**Purpose:** Implement only the accepted scope.

**Expected input:** Accepted proposal (or accepted update) with required files present under `openspec/changes/<change-id>/`; allowed paths/actions; success criteria.

**Expected output:** Code/docs changes within scope; file list; evidence of intended checks run or explicitly not-run; blockers.

**Allowed:** Mutate agreed surfaces; run local checks needed for the accepted work; report limits.

**Forbidden:** Implement without verifying `proposal.md` + `tasks.md` (and required specs/design) exist and are sufficient; expand scope; “while here” refactors; silent Git commit/push/PR; invent test results; invent OpenSpec CLI output.

**Ask when:** A material conflict, missing dependency, or safer alternative appears.

**Stop when:** Conflict invalidates acceptance, authorization is missing, or continuing needs invented evidence.

**Human approval:** Required to expand scope or perform delivery/Git actions.

**Minimum evidence:** Changed files; mapping to accepted tasks; checks run/not-run with outcomes.

### 4.7 Verify (`atlas-sdd-verify`)

**Purpose:** Validate the result honestly.

**Expected input:** Implementation claim; accepted validation plan; runnable checks.

**Expected output:** Pass / fail / not-run per check with evidence; residual risks; no success declaration without evidence.

**Allowed:** Run permitted checks; inspect diffs/logs; report gaps.

**Forbidden:** Declare success without evidence; silently fix unless explicitly delegated; invent PASS; Git delivery; OpenSpec CLI fabrication.

**Ask when:** A required check cannot run and policy for waiver is unclear.

**Stop when:** Evidence is insufficient and further claims would be false.

**Human approval:** Required to waive failed/required checks or to authorize fix-forward work.

**Minimum evidence:** Check table with status and command/inspection source; limitations.

### 4.8 Archive (`atlas-sdd-archive`)

**Purpose:** Record final outcome, evidence, decisions, and debt. Not merge authority.

**Expected input:** Verify report; decision log; remaining debt.

**Expected output:** Closeout note: outcome, evidence pointers, decisions, debt, recommended next step. No commit/push/PR/merge performed by this phase.

**Allowed:** Write archive/closeout notes in agreed locations; recommend next human actions.

**Forbidden:** Commit, push, open PR, merge, tag, release; rewrite history; claim delivery completed without human delivery actions.

**Ask when:** Outcome classification (done / deferred / abandoned) is unclear.

**Stop when:** Archive would require inventing verification or delivery results.

**Human approval:** Required for any Git/delivery action after archive recommendation.

**Minimum evidence:** Final status; evidence references; decision list; debt list; next-step recommendation.

## 5. Authority boundary: OpenSpec vs Atlas vs project docs

| Surface | Owns |
|---------|------|
| OpenSpec | Authoritative change artifacts: proposal, specs, design, tasks, verify output, archive |
| Atlas | Governance, normalization, lifecycle observation, traceability references, Status/Doctor |
| Project docs | Real product documentation (API, architecture/ADR, operations) |
| Memory | Distilled durable knowledge via read-only `atlas memory` retrieval and the `memory` skill — not a copy of OpenSpec archives; proposals are not Memory observations |

Atlas does **not** create a parallel evidence store (no `.atlas/evidence/`, no duplicated OpenSpec manifests). Atlas may read and reference OpenSpec changes; it must not reinterpret or reconstruct their contents as Atlas-owned copies.

### 5.1 Project documentation policy

Update project documentation only when the change itself requires it (for example API change → API docs; architectural change → architecture/ADR; operational change → deployment/ops docs).

Do **not** create documentation solely to leave evidence that Atlas executed a phase. SDD history belongs in OpenSpec; Memory may distill durable knowledge from archived change references without copying archives. Quality-governed automatic persistence is available via `atlas memory propose` — not by logging phase transcripts.

## 6. Cross-cutting rules

### 6.1 When to ask

Ask when scope, authority, irreversible action, acceptance, or material risk is unclear. Prefer one precise question over silent invention.

### 6.2 When to stop

Stop when:

- authorization is missing
- request conflicts with `AGENTS.md` or this contract
- Atlas-owned surfaces needed for governance are missing/drifted and Runtime Repair is required
- continuing would invent evidence, approvals, or OpenSpec results
- accepted scope would be exceeded

### 6.3 When to request human approval

Always for: proposal acceptance before Implement; scope expansion; Git/delivery; remote/publish side effects; waiving required verification; treating review findings as override of human/AGENTS authority.

### 6.4 Minimum evidence standard

Every phase output must separate:

- **Observed** (inspected/ran)
- **Inferred** (reasoned)
- **Recommended** (advisory)
- **Not run / unknown**

No silent success.

### 6.5 Scope control

Stay inside accepted/requested scope. Propose expansions; do not perform them. Worker and review outputs cannot widen scope.

### 6.6 No silent Git

Do not commit, amend, rebase, reset, push, force-push, tag, publish, open/merge PRs, or otherwise deliver unless the human explicitly requests that action in the current session. Passing checks ≠ delivery approval.

### 6.7 Skills policy

Skills are canonical under Atlas Home and indexed via `.atlas/skill-registry.md`. Adapter skill folders are regenerable projections only. Do not download or invent skills.

### 6.8 Relation to AGENTS.md

`AGENTS.md` wins on conflict. Adapter entrypoints and agent files are execution surfaces under `AGENTS.md` and this contract.

### 6.9 Relation to agent-registry.md

Route using `.atlas/agent-registry.md` and prefer `atlas-orchestrator`. Do not invent agents outside the registry. Developer-owned non-Atlas agents remain outside Atlas ownership.

### 6.10 Relation to skill-registry.md

Consult `.atlas/skill-registry.md` when present. Missing entries mean unavailable—say so; do not invent skills.

### 6.11 Relation to OpenSpec as source of truth

When an OpenSpec project tree is present, OpenSpec owns the change artifacts. Atlas Status/Doctor may observe lifecycle and emit normalized references. Do not duplicate those artifacts into Atlas storage.

Before **Implement**, the active change under `openspec/changes/<change-id>/` MUST include at least `proposal.md` and `tasks.md`, plus `specs/<domain>/spec.md` when domain requirements apply, and `design.md` when an explicit design is required. Those files MUST unambiguously identify:

- change identity (`<change-id>`)
- intent/problem
- scope
- requirements/acceptance criteria
- approved design/approach when applicable
- implementation tasks
- human acceptance when this contract requires it

Rules:

- OpenSpec CLI may be used when available; it is not the only allowed materialization path
- do not install OpenSpec as a side effect
- do not invent command output or pretend `proposal.md` / `tasks.md` / specs exist
- do not copy proposal/spec/design/tasks/verify/archive contents into `.atlas/` as a parallel store
- creating the layout above via the governed agent path is allowed and expected when CLI is absent
- requesting a feature is **not** acceptance of a proposal that did not yet exist

## 7. Reviewer stance

`atlas-review-architecture`, `atlas-review-risk`, `atlas-review-quality`, and `atlas-review-refuter` are reviewers / adversarial reviewers. They produce findings and questions. They never authorize delivery, acceptance, or scope changes.

## 8. Worker stance

`atlas-worker` executes one bounded mission under orchestrator/human authority. It must return evidence and limits, must not expand scope, and must stop when the mission would require new authority.
