module github.com/jarviisha/codohue/examples/bskyfeed

go 1.26.7

// Local development: resolve the shared wire types and the Redis stream
// producer from this repo via the go.work workspace. Keeping this feeder in
// its own module is what stops its websocket dependency from reaching the
// server's go.mod — it is a development data source, not part of the service.
replace (
	github.com/jarviisha/codohue/pkg/codohuetypes => ../../pkg/codohuetypes
	github.com/jarviisha/codohue/sdk/go/redistream => ../../sdk/go/redistream
)

require (
	github.com/coder/websocket v1.8.14
	github.com/jarviisha/codohue/pkg/codohuetypes v0.7.0
	github.com/jarviisha/codohue/sdk/go/redistream v0.0.0-00010101000000-000000000000
	github.com/redis/go-redis/v9 v9.18.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	go.uber.org/atomic v1.11.0 // indirect
)
