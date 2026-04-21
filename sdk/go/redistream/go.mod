module github.com/jarviisha/codohue/sdk/go/redistream

go 1.24.13

// Local development: resolve pkg/codohuetypes from the repo. The replace
// directive is scoped to this module's own builds and does not propagate to
// downstream consumers. On release, bump the require below to the matching
// pkg/codohuetypes tag and keep the replace for local dev via go.work.
replace github.com/jarviisha/codohue/pkg/codohuetypes => ../../../pkg/codohuetypes

require (
	github.com/jarviisha/codohue/pkg/codohuetypes v0.0.0-00010101000000-000000000000
	github.com/redis/go-redis/v9 v9.18.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	go.uber.org/atomic v1.11.0 // indirect
)
