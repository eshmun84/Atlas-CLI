## OpenCode Adapter Guidance

OpenCode must treat root `AGENTS.md` as the project authority for this repository.

`.opencode/atlas.md` is only the OpenCode-native entrypoint. It exists to point OpenCode at this contract. It is not an independent policy source and must not duplicate or dilute the full Atlas contract.

OpenCode agents, workers, reviewers, commands, and plugins are execution surfaces, not independent authorities. Delegated work produces evidence; it does not grant delivery approval.

OpenCode must not bypass Atlas delivery or Git rules. Explicit human request is still required for commit, push, publish, and other irreversible delivery actions. If OpenCode guidance conflicts with this file, this file wins.
