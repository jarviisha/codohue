# Escaped-id orphans (path-decoding fix)

Before route parameters were decoded at the boundary, `did%3Aplc%3Aabc` and
`did:plc:abc` were two different entities. A client that percent-escaped
reserved characters in an id therefore created records under the escaped
string. After the upgrade that same client addresses the decoded id, and the
old records are orphaned.

This is a data cleanup, not a rollout step. Most deployments need nothing.

## Who is affected

Only the three write routes that take an `{id}` and mint an identity:

| Route | Records created under the escaped string |
| ----- | ---------------------------------------- |
| `PUT /v1/namespaces/{ns}/objects/{id}` | `id_mappings` row, `objects` row |
| `PUT /v1/namespaces/{ns}/objects/{id}/embedding` | `id_mappings` row, point in `{ns}_objects_dense` |
| `PUT /v1/namespaces/{ns}/subjects/{id}/embedding` | `id_mappings` row, point in `{ns}_subjects_dense` |

Not affected:

- **Reads.** `GET …/recommendations` resolves through `LookupSubjectID`, which
  never creates a row. An escaped read returned a fallback and wrote nothing.
- **`POST /v1/namespaces/{ns}/events`** and the catalog routes. Subject and
  object ids travel in the JSON body, which was never percent-encoded.
- **Namespaces on `dense_source="catalog"`.** Object BYOE returns 409 there, so
  only the metadata PUT and subject embeddings could have run.

Why it matters: an orphaned dense point stays in its collection and is still
returned by vector search, so a recommendation can surface an `object_id` that
is the escaped string — an id the client cannot resolve back to its content.

## 1. Detect

Candidate rows are those whose id contains a percent-escape:

```sql
SELECT namespace, entity_type, string_id, numeric_id
FROM id_mappings
WHERE string_id ~ '%[0-9A-Fa-f]{2}'
ORDER BY namespace, entity_type, string_id;
```

An empty result means this deployment is unaffected — stop here.

A non-empty result is **not** proof of a problem: an id may legitimately
contain a literal `%`. Confirm by checking whether the decoded twin also
exists, which is what makes the escaped row a duplicate identity rather than a
distinct one:

```sql
SELECT e.namespace, e.entity_type, e.string_id AS escaped, d.string_id AS decoded
FROM id_mappings e
JOIN id_mappings d
  ON  d.namespace   = e.namespace
  AND d.entity_type = e.entity_type
  AND d.string_id   = replace(replace(replace(e.string_id,
        '%3A', ':'), '%2F', '/'), '%3F', '?')
WHERE e.string_id ~ '%[0-9A-Fa-f]{2}';
```

Extend the `replace` chain for any other reserved character your clients
escape. Rows returned here are the confirmed orphans.

## 2. Clear

There is no automatic migration: only the client knows whether an escaped id
was a mistake or a real id containing `%`. For each confirmed orphan, the
decoded identity is authoritative.

1. Take a PostgreSQL backup and a Qdrant snapshot of the affected collections
   at the same checkpoint.
2. Delete the orphan through the API so Postgres and Qdrant stay consistent —
   `DELETE /v1/namespaces/{ns}/objects/{id}` removes its points from
   `{ns}_objects` and `{ns}_objects_dense` and drops its `objects` row. Send
   the escaped spelling **double-escaped** (`%253A` where the orphan's literal
   id holds `%3A`) so the decoded parameter is the orphan's own string.
3. Re-push the record under the decoded id if the client has not already: the
   BYOE vector, or the `author_subject_id` metadata.
4. Re-run the detection query.

The delete route intentionally leaves the `id_mappings` row: it is a
name-to-number binding with no vectors and no metadata behind it, so it cannot
surface in a response, and the decoded id has its own separate row. The
detection query therefore still reports cleared orphans. To confirm one is
inert, check that nothing references its `numeric_id`; drop the row only if you
want the query to come back empty.

Subject orphans have no delete route. Remove the point from
`{ns}_subjects_dense` by its `numeric_id` and delete the `id_mappings` row, or
leave it: subject vectors are rebuilt each cron tick from `events`, which never
carried escaped ids, so the orphan simply stops being refreshed.
