// Command loadgen is a runnable example client for Codohue. It bootstraps a
// namespace through the admin plane (login, upsert config, enable catalog,
// seed catalog content) and then continuously pumps a realistic stream of
// behavioral events through the public HTTP ingest path using the Go SDK.
//
// While it runs it periodically reads recommendations and trending back out,
// so you can watch the recommendation loop close as the cron and embedder
// workers process the data it produces.
package main
