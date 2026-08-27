# Data-plane Contract Changes

All changes are additive or convert false success into an explicit failure. `web/admin` is not
changed.

## Namespace resolution

Every namespace-scoped mutation, including calls authenticated by the global admin key,
requires an active namespace lifecycle and config.

| Condition | HTTP status | Stable error code |
|-----------|-------------|-------------------|
| Namespace absent/deleted | 404 | `namespace_not_found` |
| Namespace deleting/system resetting | 409 | `namespace_not_active` |
| Config/lifecycle store unavailable | 503 | `namespace_config_unavailable` |

Recommendation, ranking, and trending resolve config before reading cache. A 503 response is
not cached, and stale cache content is not served when current config cannot be established.

## Timestamp validation

Event `object_created_at` and BYOE object `object_created_at` may be at most five minutes ahead
of server UTC time. Values beyond the boundary return 400 with `invalid_object_created_at`.
The exact boundary is accepted.

Every successful recommendation/ranking response contains finite JSON numbers. Future legacy
payloads cannot boost freshness above the base score; malformed payload timestamps contribute
no freshness adjustment.

## Object deletion

`DELETE /v1/namespaces/{ns}/objects/{id}` remains idempotent.

- 204 means sparse point, dense point, and object metadata are absent or were removed.
- Missing optional collections/points are success.
- Any other cleanup failure returns a retryable 5xx error after all cleanup stages have been
  attempted.
- Retrying after partial cleanup converges safely.

## Atomic namespace configuration

A namespace upsert containing catalog strategy fields validates the effective final dimension
and strategy before commit. Base config and catalog config commit together or not at all. The
existing request/response JSON remains unchanged apart from the additive generation returned
for provisioning clients.

## Atomic catalog attribution

When catalog ingest includes `author_subject_id`, catalog content and object attribution commit
together. Same-content re-ingest may update attribution without republishing embedding work.
An attribution failure cannot produce an accepted item response.

## Catalog reconciliation

Existing endpoint:

```http
GET /v1/namespaces/{ns}/catalog/objects
```

Preferred request:

```text
?changed_since=<RFC3339>&limit=<1..1000>&cursor=<opaque>
```

Additive response field:

```json
{
  "items": [{"object_id":"item-1","updated_at":"2026-08-24T00:00:00Z"}],
  "limit": 100,
  "next_cursor": "eyJ2IjoxLCJ0IjoiLi4uIiwiaWQiOjEyM30"
}
```

`next_cursor` is omitted on the terminal page. A malformed or mismatched cursor returns 400.
Legacy `offset` remains accepted for one compatibility window but is documented as unsuitable
for a live repair walk.

## Health and metrics

Public `/healthz` keeps 200/503 and its current component/status keys, but values are only
`ok` or `error`; it never returns dependency addresses or raw errors.

Protected diagnostics use the same listener and an explicit request:

```http
GET /healthz?details=true
Authorization: Bearer <observability-token>
```

With a valid token, the response retains the public status fields and adds a `details` object
containing per-dependency diagnostic messages. Missing/incorrect credentials return 401. When no
observability token is configured, `details=true` returns 404. Plain `/healthz` remains public
and sanitized even when an invalid Authorization header is present unless `details=true` is
requested.

`/metrics` is registered only when `CODOHUE_OBSERVABILITY_TOKEN` is configured. It requires:

```http
Authorization: Bearer <observability-token>
```

Missing/incorrect credentials return 401. When no token is configured the route is absent
(404). The token is distinct from the global admin API key and is compared in constant time.
The same public/protected health behavior and metrics rule apply to the API and embedder
listeners.
