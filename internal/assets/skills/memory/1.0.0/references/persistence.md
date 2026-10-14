# Memory persistence (propose)

Internal agent transport:

```text
atlas memory propose --json < candidate.json
```

## Candidate fields

| Field | Required | Notes |
|-------|----------|-------|
| `type` | yes | V1 taxonomy type |
| `title` | yes | concrete, bounded |
| `content` | yes | durable knowledge body |
| `scope` | no | must be `project` (default) |
| `topic_key` | no | stable subject for supersede |
| `origin` | yes | `atlas_lifecycle` \| `developer_confirmation` \| `atlas_operation` |
| `explicitness` | no | `explicit` (writable) \| `confirmed` (reserved/no-write) \| `inferred` \| `speculative` |
| `confidence` | no | `high` \| `medium` \| `low` |
| `provenance` | no | bounded opaque ref — never absolute Home paths |
| `source_event` | no | bounded event id |

Stdin must be exactly one JSON object. Unknown fields (including any `confirm` key) are rejected. Self-confirming inferred candidates is forbidden.

## Statuses

`accepted`, `duplicate`, `updated`, `rejected`, `confirmation_required`, `disabled`, plus failure codes.

Soft outcomes (`rejected`, `confirmation_required`, `duplicate`, `disabled`) must not block development.

Do not submit arbitrary assistant prose. Prefer `explicit` only. Inferred/confirmed → `confirmation_required` (no write).
