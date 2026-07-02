# Contract: Namespace configuration surface

Scope: admin-plane configuration DTOs only. The SDK-facing wire contract in
`pkg/codohuetypes` is **unaffected** (see research.md R1).

## Affected endpoints (paths unchanged)

- `PUT /api/admin/v1/namespaces/{ns}` — create/update namespace config
- `GET /api/admin/v1/namespaces/{ns}` and `GET /api/admin/v1/namespaces` — read config
- `PUT /api/admin/v1/namespaces/{ns}/catalog` — enable/update/disable catalog
- `GET /api/admin/v1/namespaces/{ns}/catalog` — read catalog config

No paths are added or removed. Only field shapes and the error surface change.

## Field change

### Request / response: namespace config

Removed:
```
"dense_strategy": "byoe" | "item2vec" | "svd" | "disabled"
"catalog_enabled": true | false
```

Added:
```
"dense_source": "disabled" | "item2vec" | "svd" | "byoe" | "catalog"
```

`alpha`, `embedding_dim`, `dense_distance`, and all `catalog_strategy_*` fields are unchanged.

### `PUT /catalog` semantics

- Before: `{ "enabled": true, "strategy_id", "strategy_version", "params" }` plus a precondition
  that the namespace's `dense_strategy ∈ {byoe, disabled}`.
- After: enabling catalog is expressed as `dense_source = "catalog"`. The catalog endpoint MAY
  continue to accept the strategy params and set `dense_source='catalog'` as part of the same
  call (it owns the catalog_strategy_* columns). The precondition disappears.
- Disable: with the `enabled` boolean gone, disabling is an explicit producer change — the caller
  sets `dense_source` to a non-catalog value (`disabled`, `byoe`, `item2vec`, or `svd`). There is
  **no implicit default**; a `PUT /catalog` that intends to turn catalog off MUST name the new
  `dense_source`. Request body becomes `{ "dense_source": "<value>", ...strategy params if catalog... }`.

## Error surface change

Removed error (no longer reachable):
```
400 { "error": "dense_strategy must be byoe or disabled when catalog_enabled=true",
      "code": "dense_strategy_conflict",
      "dense_strategy": "...", "catalog_enabled": true }
```

Retained errors:
```
400  dimension mismatch — { "error": "strategy dimension mismatch",
                            "strategy_dim": N, "namespace_embedding_dim": M }
400  invalid dense_source value (CHECK / guard rejection)
404  namespace not found
503  catalog feature not wired (configurator/picker absent)
```

## Backward-compatibility note

Admin UI and any admin API consumers MUST switch from reading `dense_strategy` + `catalog_enabled`
to reading `dense_source`. There is no transitional period where both shapes are served on the
admin API; the dual-write window (research.md R3) is at the **database** layer, not the API layer.

## Contract tests to update

- Any admin handler/golden test under `internal/admin` that asserts the presence of
  `dense_strategy` / `catalog_enabled` in config payloads → assert `dense_source`.
- Tests asserting the `dense_strategy_conflict` 400 → delete.
- `pkg/codohuetypes` golden suite → **no change** (fields not present there).
