# Memory retrieval CLI

Always pass `--json`. Working directory must be inside an initialized Atlas project.

## Progressive context (preferred)

```text
atlas memory context "<task description>" --json [--limit N] [--budget N]
```

- `--limit`: max items (default 8, max 32)
- `--budget`: max charged UTF-8 runes across returned items (default 4000, max 32000)
- Charged fields per item: `id` + `type` + `scope` + `title` + `topic_key` + `excerpt`
- `score` and `ref` are not charged

Useful fields: `items[]` (`id`, `type`, `scope`, `title`, `topic_key`, `score`, `excerpt`, `ref.get`), `used_chars`, `budget_chars`, `limits[]`.

`ref.get` is the observation id for `atlas memory get <id> --json`.

## Search

```text
atlas memory search "<query>" --json [--limit N]
```

Compact `results[]` with `preview` (not full body). Default limit 20, max 100.

## Get

```text
atlas memory get <observation-id> --json
```

Returns full `observation` DTO when found.

## Related composition

For multi-file orientation combining Memory + Context Economy:

```text
atlas context compose "<task>" --json [--budget N] [--limit N]
```

Read-only; never persists Memory. Prefer Memory `context`/`get` for narrower queries.
