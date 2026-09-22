# Grouped namespace settings rollout

1. Apply migration 029 before serving the grouped configuration API. It is additive;
   existing writers advance revisions through the trigger without code changes.
2. Deploy the compatible admin backend, then the updated admin SPA. Local testing
   keeps the web frontend in Vite dev mode on port 2006.
3. Read `/api/admin/v1/namespaces/{ns}/configuration` using an operator session.
   Check four groups, generation, revisions, locks and default observations.
4. Validate candidates using `/configuration/validation` before integration smoke
   writes. Verify independent group saves and stale same-group conflicts using
   disposable namespaces, never production namespace configuration.
5. Batch and re-embedding remain explicit operations. Saving source/configuration
   does not delete collections, rebuild vectors or enqueue jobs.

For application rollback, restore the previous admin/SPA while retaining revision
columns and trigger. Do not roll migration 029 down while grouped API clients are
served. Legacy PUT callers remain last-write-wins and can invalidate a Settings
baseline; migrate those clients to PATCH for concurrency protection.
