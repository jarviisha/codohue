module github.com/jarviisha/codohue

go 1.26.1

// Shared wire types live in a sibling module so SDK consumers do not pull in
// the server's dependency graph (pgx, qdrant, prometheus, etc).
replace github.com/jarviisha/codohue/pkg/codohuetypes => ./pkg/codohuetypes

require (
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/go-chi/chi/v5 v5.2.5
	github.com/jackc/pgx/v5 v5.9.1
	github.com/jarviisha/codohue/pkg/codohuetypes v0.0.0-00010101000000-000000000000
	github.com/joho/godotenv v1.5.1
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/qdrant/go-client v1.17.1
	github.com/redis/go-redis/v9 v9.18.0
	golang.org/x/crypto v0.50.0
	golang.org/x/text v0.36.0
	gonum.org/v1/gonum v0.17.0
	google.golang.org/grpc v1.78.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209200024-4cfbd4190f57 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
