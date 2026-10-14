# Memory diagnostics (read-only)

When Memory retrieval or persistence appears broken:

- `atlas status` — Memory summary (status / storage / schema / retrieval / persistence / observations / quality)
- `atlas doctor` — deep Memory checks

Diagnostics never create the Memory DB, migrate schema, repair records, or write observations.

Do not treat health output as project policy. Do not invent a separate diagnostics CLI.

## Status vocabulary

| Status | Meaning |
|--------|---------|
| `success` | Hits / observation returned |
| `no_results` | Query ran; nothing relevant |
| `accepted` / `duplicate` / `updated` | Propose write outcome |
| `rejected` / `confirmation_required` / `disabled` | Propose soft no-write |
| `missing_memory_db` | No Memory DB yet |
| `not_initialized` | Project / Atlas Home not ready |
| `not_found` | Unknown observation id |
| `invalid_query` / `invalid_usage` / `invalid_input` | Bad input |
| `unsafe_filesystem` / `incompatible_schema` / `unavailable` / `internal_error` | Fail closed; report and continue |

Missing or empty Memory is normal early in a project. Continue the task.
