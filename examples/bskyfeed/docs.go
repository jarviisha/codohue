// Command bskyfeed continuously pumps live public activity from the Bluesky
// Jetstream firehose into a Codohue namespace, so a development stack has a
// real, always-fresh behavioral dataset instead of synthetic traffic.
//
// It maps AT Protocol records onto the Codohue wire contract:
//
//	app.bsky.feed.like    → LIKE    event  (actor DID → liked post URI)
//	app.bsky.feed.repost  → SHARE   event  (actor DID → reposted post URI)
//	app.bsky.feed.post    → catalog item, plus a COMMENT event when the post
//	                        is a reply (actor DID → parent post URI)
//
// Events go to the durable codohue:events stream and catalog content to
// codohue:catalog, both through the redistream SDK, so the feeder does not
// depend on cmd/api being reachable and survives an ingest outage.
//
// Configuration is environment-only (see README.md); the service is toggled
// through the "bskyfeed" Compose profile rather than a flag.
package main
