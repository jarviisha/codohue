// Package admin is a bearer-authenticated client for Codohue's admin plane
// (cmd/admin, port 2002). It exists so provisioning is a supported call
// instead of a hand-rolled session-cookie dance in every consumer: the admin
// server accepts `Authorization: Bearer <CODOHUE_ADMIN_API_KEY>` on
// /api/admin/v1/* directly.
//
// The paved road is ProvisionCatalogNamespace — one request that creates or
// updates a namespace in the system's core mode (dense_source="catalog") with
// the embedding strategy validated against embedding_dim server-side.
package admin
