# Quickstart: Test Pipeline Controls Locally

## Prerequisites

```bash
make up-infra          # start postgres, redis, qdrant
make dev               # start api (port 2001) in another terminal
# start admin server (port 2002) in another terminal
```

Ensure at least one namespace exists (create via the Namespaces page or `PUT /v1/config/namespaces/{ns}`).

---

## 1. Inject a Test Event

Via the UI:
1. Open http://localhost:5173/events
2. Select your namespace
3. Fill: Subject ID = `user-1`, Object ID = `item-42`, Action = `VIEW`
4. Click Submit

Via curl (alternative):
```bash
curl -X POST http://localhost:2002/api/admin/v1/namespaces/my_feed/events \
  -H "Cookie: codohue_admin_session=<your-session>" \
  -H "Content-Type: application/json" \
  -d '{"subject_id":"user-1","object_id":"item-42","action":"VIEW"}'
# expect: 202 {"ok":true}
```

---

## 2. Confirm Event in List

```bash
curl "http://localhost:2002/api/admin/v1/namespaces/my_feed/events?limit=5" \
  -H "Cookie: codohue_admin_session=<your-session>"
# expect: events array with the injected event at index 0
```

---

## 3. Trigger Batch Run

Via the UI: Click "Run now" on the namespace card in the Namespaces page.

Via curl:
```bash
curl -X POST http://localhost:2002/api/admin/v1/namespaces/my_feed/batch-runs/trigger \
  -H "Cookie: codohue_admin_session=<your-session>"
# expect: 200 {"batch_run_id":N,"success":true,"duration_ms":...}
```

Check the Batch Runs page — the new run should appear with phase breakdown.

---

## 4. Verify Recommendations

In the Recommend Debug page, enter:
- Namespace: `my_feed`
- Subject ID: `user-1`

Results should reflect the injected interaction after the batch run.

---

## Error Scenarios

| Scenario | Expected |
|----------|----------|
| Trigger batch while one is running | `409 {"error":"batch already in progress for namespace my_feed"}` |
| Inject event with empty subject_id | `400 {"error":"subject_id is required"}` |
| Events list with invalid limit | `400 {"error":"limit must be between 1 and 200"}` |
