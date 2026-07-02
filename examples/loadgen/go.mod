module github.com/jarviisha/codohue/examples/loadgen

go 1.26.1

// Local development: resolve the SDK and shared wire types from this repo via
// the go.work workspace. The replace directives keep the module graph
// self-consistent so `go build`/gopls don't try to fetch unpublished tags.
replace (
	github.com/jarviisha/codohue/pkg/codohuetypes => ../../pkg/codohuetypes
	github.com/jarviisha/codohue/sdk/go => ../../sdk/go
)

require (
	github.com/jarviisha/codohue/pkg/codohuetypes v0.3.0
	github.com/jarviisha/codohue/sdk/go v0.0.0-00010101000000-000000000000
)
