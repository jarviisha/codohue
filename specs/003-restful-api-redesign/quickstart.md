# Quickstart: Verifying the RESTful API Redesign

**Feature**: 003-restful-api-redesign
**Audience**: Anyone reviewing or smoke-testing the change locally.

This guide walks through a happy-path verification of every consolidated route after the redesign lands. It mirrors the acceptance scenarios in [spec.md](./spec.md) and serves as a manual smoke-test before tagging a release.

---

## 0. Prerequisites

```bash
cp .env.example .env                    # if not already
make up-infra                           # postgres + redis + qdrant
make migrate-up
make build
./tmp/api &                             # data plane on :2001
./tmp/admin &                           # admin plane on :2002
./tmp/cron &                            # batch daemon
```

Then in a separate shell:

```bash
export ADMIN_KEY="dev-secret-key"       # value of RECOMMENDER_API_KEY
export NS="quickstart"
export SUBJECT="user_1"
```

Confirm health:

```bash
curl -s :2001/healthz | jq .
curl -s :2002/api/admin/v1/health -H "Cookie: $COOKIE" | jq .   # after login below
```

---

## 1. Admin login → session cookie

```bash
COOKIE=$(curl -s -c - -X POST :2002/api/v1/auth/sessions \
  -H "Content-Type: application/json" \
  -d "{\"api_key\":\"$ADMIN_KEY\"}" \
  -o /dev/null -w '%header{set-cookie}')

# Or capture cookies into a jar:
curl -s -c cookies.txt -X POST :2002/api/v1/auth/sessions \
  -H "Content-Type: application/json" \
  -d "{\"api_key\":\"$ADMIN_KEY\"}"
```

Expected: **201 Created**, `Set-Cookie: codohue_admin_session=...`, body `{"expires_at":"..."}`.

Negative check — old path should be 404:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST :2002/api/auth/login   # → 404
```

---

## 2. Admin upserts a namespace (data plane has no equivalent)

```bash
curl -s -b cookies.txt -X PUT :2002/api/admin/v1/namespaces/$NS \
  -H "Content-Type: application/json" \
  -d '{
    "alpha": 0.7,
    "dense_strategy": "byoe",
    "embedding_dim": 4,
    "dense_distance": "cosine",
    "action_weights": {
      "VIEW": 1.0,
      "LIKE": 2.0,
      "COMMENT": 3.0,
      "SHARE": 4.0,
      "SKIP": -1.0
    },
    "lambda": 0.1,
    "gamma": 0.05,
    "max_results": 20,
    "seen_items_days": 30,
    "trending_window": 24,
    "trending_ttl": 600,
    "lambda_trending": 0.1
  }' | jq .
```

Expected: **201 Created** the first time (response includes one-time `api_key`), **200 OK** thereafter.

Negative check — data plane no longer registers this route:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X PUT :2001/v1/config/namespaces/$NS \
  -H "Authorization: Bearer $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{}'    # → 404
```

Save the returned `api_key` for the namespace:

```bash
export NS_KEY="<paste from response>"
```

---

## 3. Ingest one event (canonical path, no namespace in body)

```bash
curl -s -X POST :2001/v1/namespaces/$NS/events \
  -H "Authorization: Bearer $NS_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"subject_id\": \"$SUBJECT\",
    \"object_id\": \"post_1\",
    \"action\": \"VIEW\"
  }" -o /dev/null -w '%{http_code}\n'
```

Expected: **202**.

Negative check — body with redundant `namespace` is silently accepted (the field is ignored, since the path wins):

```bash
curl -s -X POST :2001/v1/namespaces/$NS/events \
  -H "Authorization: Bearer $NS_KEY" -H "Content-Type: application/json" \
  -d "{\"namespace\":\"WRONG\",\"subject_id\":\"$SUBJECT\",\"object_id\":\"post_2\",\"action\":\"VIEW\"}" \
  -o /dev/null -w '%{http_code}\n'   # → 202, event recorded under $NS
```

---

## 4. Trigger a batch run, then fetch recommendations

```bash
LOC=$(curl -s -b cookies.txt -X POST :2002/api/admin/v1/namespaces/$NS/batch-runs \
  -D - -o /dev/null | awk '/^Location:/ { print $2 }' | tr -d '\r')
echo "Created batch run: $LOC"
```

Expected: **202**, `Location: /api/admin/v1/namespaces/$NS/batch-runs/<id>`.

Wait a few seconds, then inspect the namespace-scoped batch-run list:

```bash
curl -s -b cookies.txt :2002/api/admin/v1/namespaces/$NS/batch-runs | jq .
```

Now fetch recommendations via the canonical sub-resource path:

```bash
curl -s :2001/v1/namespaces/$NS/subjects/$SUBJECT/recommendations?limit=5 \
  -H "Authorization: Bearer $NS_KEY" | jq .
```

Expected:

```json
{
  "items": [...],
  "total": 5,
  "source": "cf" | "trending" | "cf_hybrid" | "cold_start",
  "generated_at": "2026-05-07T..."
}
```

Negative checks — every legacy form returns 404:

```bash
for path in \
  "/v1/recommendations?namespace=$NS&subject_id=$SUBJECT" \
  "/v1/namespaces/$NS/recommendations?subject_id=$SUBJECT" \
  "/v1/trending/$NS"; do
  echo -n "$path → "
  curl -s -o /dev/null -w '%{http_code}\n' "http://localhost:2001$path" \
    -H "Authorization: Bearer $NS_KEY"
done
```

Each should print **404**.

---

## 5. Compute rankings (no namespace in body)

```bash
curl -s -X POST :2001/v1/namespaces/$NS/rankings \
  -H "Authorization: Bearer $NS_KEY" -H "Content-Type: application/json" \
  -d "{\"subject_id\":\"$SUBJECT\",\"candidates\":[\"post_1\",\"post_2\",\"post_3\"]}" | jq .
```

Expected: **200** with ranked items.

Negative check:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST :2001/v1/rank \
  -H "Authorization: Bearer $NS_KEY" -H "Content-Type: application/json" \
  -d "{\"namespace\":\"$NS\",\"subject_id\":\"$SUBJECT\",\"candidates\":[\"post_1\"]}"
# → 404
```

---

## 6. Store a BYOE embedding (PUT, idempotent)

```bash
curl -s -X PUT :2001/v1/namespaces/$NS/objects/post_1/embedding \
  -H "Authorization: Bearer $NS_KEY" -H "Content-Type: application/json" \
  -d '{"vector":[0.1, 0.2, 0.3, 0.4]}' \
  -o /dev/null -w '%{http_code}\n'
# → 204

# Repeat the same call — must still return 204.
curl -s -X PUT :2001/v1/namespaces/$NS/objects/post_1/embedding \
  -H "Authorization: Bearer $NS_KEY" -H "Content-Type: application/json" \
  -d '{"vector":[0.1, 0.2, 0.3, 0.4]}' \
  -o /dev/null -w '%{http_code}\n'
# → 204 (idempotent)
```

Negative check — the old POST form is gone:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST :2001/v1/objects/$NS/post_1/embedding \
  -H "Authorization: Bearer $NS_KEY" -H "Content-Type: application/json" \
  -d '{"vector":[0,0,0,0]}'
# → 404 (or 405 if the path itself still exists with PUT only — confirm it's 404)
```

---

## 7. Delete an object (idempotent)

```bash
curl -s -X DELETE :2001/v1/namespaces/$NS/objects/post_1 \
  -H "Authorization: Bearer $NS_KEY" -o /dev/null -w '%{http_code}\n'
# → 204

# Repeat — still 204.
```

---

## 8. Admin debug recommendations (query-mode)

```bash
curl -s -b cookies.txt \
  ":2002/api/admin/v1/namespaces/$NS/subjects/$SUBJECT/recommendations?debug=true&limit=3" | jq .
```

Expected: **200** with `items`, `total`, `source`, `generated_at`, plus a `debug` block.

Negative check:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -b cookies.txt \
  -X POST :2002/api/admin/v1/recommend/debug \
  -H "Content-Type: application/json" \
  -d "{\"namespace\":\"$NS\",\"subject_id\":\"$SUBJECT\"}"
# → 404
```

---

## 9. Admin Qdrant inspection

```bash
curl -s -b cookies.txt :2002/api/admin/v1/namespaces/$NS/qdrant | jq .
```

Expected:

```json
{
  "subjects":       { "exists": true, "points_count": ... },
  "objects":        { "exists": true, "points_count": ... },
  "subjects_dense": { ... },
  "objects_dense":  { ... }
}
```

Negative check:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -b cookies.txt \
  :2002/api/admin/v1/namespaces/$NS/qdrant-stats
# → 404
```

---

## 10. Demo data lifecycle

```bash
curl -s -b cookies.txt -X POST   :2002/api/admin/v1/demo-data -o /dev/null -w '%{http_code}\n'  # → 202
curl -s -b cookies.txt -X DELETE :2002/api/admin/v1/demo-data -o /dev/null -w '%{http_code}\n'  # → 204
```

Negative check:

```bash
for m in POST DELETE; do
  echo -n "$m /demo → "
  curl -s -o /dev/null -w '%{http_code}\n' -b cookies.txt -X $m :2002/api/admin/v1/demo
done
# both → 404
```

---

## 11. Logout

```bash
curl -s -b cookies.txt -X DELETE :2002/api/v1/auth/sessions/current -o /dev/null -w '%{http_code}\n'
# → 204
```

Negative check:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE :2002/api/auth/logout
# → 404
```

---

## 12. Web UI walk-through

After backend smoke-tests pass, open the admin UI in a browser:

```bash
open http://localhost:2002/      # macOS
xdg-open http://localhost:2002/  # Linux
```

Verify each screen loads and triggers the correct calls (open DevTools Network tab):

- **Login screen** → `POST /api/v1/auth/sessions`
- **Overview** → `GET /api/admin/v1/namespaces?include=overview`, `GET /api/admin/v1/health`
- **Namespace detail** → `GET /api/admin/v1/namespaces/{ns}`, `GET .../qdrant`, `GET .../batch-runs`, `GET .../events`
- **Trigger batch** button → `POST .../batch-runs` (returns 202)
- **Inject test event** → `POST .../events`
- **Debug recommendations** form → `GET .../subjects/{id}/recommendations?debug=true`
- **Demo seed/clear** → `POST` and `DELETE /api/admin/v1/demo-data`
- **Logout** → `DELETE /api/v1/auth/sessions/current`

No browser console error. No 404 on any non-test action.

---

## 13. Pass / Fail criteria

The redesign is verified when:

- Every command in sections 1–11 returns the expected status code.
- Every "Negative check" returns **404** (or **405** where explicitly noted).
- Section 12 walk-through completes with zero broken pages.
- `make test` passes.
- `make lint` passes.
- The CLAUDE.md REST API table reflects the new surface only.
