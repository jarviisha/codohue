module github.com/jarviisha/codohue/sdk/go

go 1.24.13

// Local development: resolve pkg/codohuetypes from this repo. This replace
// directive is scoped to this module's own builds and does not propagate to
// downstream consumers. On release, bump the require below to the matching
// pkg/codohuetypes tag and keep the replace for local dev via go.work.
replace github.com/jarviisha/codohue/pkg/codohuetypes => ../../pkg/codohuetypes

require github.com/jarviisha/codohue/pkg/codohuetypes v0.2.0
