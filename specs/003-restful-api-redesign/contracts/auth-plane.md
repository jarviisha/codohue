# Auth-Plane API Contract

**Server**: `cmd/admin`, port `2002`
**Auth**: none for `POST /api/v1/auth/sessions` (it issues the session). `DELETE /api/v1/auth/sessions/current` requires the session cookie.
**Cookie**: `codohue_admin_session`, `HttpOnly`, `Secure` (when TLS is on), `SameSite=Lax`, scoped to `/api` so it is sent to both auth-plane and admin-plane routes.

> Sessions are modeled as a resource. Login is "create session"; logout is "delete current session".

---

## `POST /api/v1/auth/sessions`

Create a new session by validating the global admin API key.

**Request body**:
```json
{ "api_key": "dev-secret-key" }
```

**Responses**:
- **201 Created**:
  - `Set-Cookie: codohue_admin_session=<token>; Path=/api; HttpOnly; SameSite=Lax`
  - Body:
    ```json
    { "expires_at": "2026-05-08T10:30:00Z" }
    ```
- **401 Unauthorized** — `api_key` does not match `RECOMMENDER_API_KEY`.
- **400 Bad Request** — body missing or malformed.

> Replaces `POST /api/auth/login`.

---

## `DELETE /api/v1/auth/sessions/current`

End the current session (the one identified by the inbound cookie).

**Request body**: none.

**Responses**:
- **204 No Content**:
  - `Set-Cookie: codohue_admin_session=; Max-Age=0; Path=/api; ...`
  - No body.
- **401 Unauthorized** — no session cookie present (treated as already logged out by some clients; we still return 401 to surface programming errors).

> Replaces `DELETE /api/auth/logout`.

---

## Why `/auth/sessions/current` and not `/auth/sessions/{id}`?

The admin UI is a single-tab single-user tool. Sessions are not enumerable from the client; the only session a client can address is its own. `current` is the conventional alias used by other REST APIs (e.g., GitHub `/user`, GitLab `/user`). If multi-session management is ever needed, `/auth/sessions/{id}` slots in as a sibling without breaking the `current` alias.

---

## Removed routes (return 404)

| Method | Removed path |
|--------|--------------|
| POST   | `/api/auth/login` |
| DELETE | `/api/auth/logout` |
