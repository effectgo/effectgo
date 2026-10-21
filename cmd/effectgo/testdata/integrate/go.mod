// Fixture module of the integration tests (cmd/effectgo/integrate_test.go).
// The Huma and Gin packages are the adapters' fakes, replaced by local paths,
// so the module needs no download.
module example.com/integrate

go 1.26

require (
	github.com/danielgtaylor/huma/v2 v2.37.2
	github.com/gin-gonic/gin v1.12.0
	github.com/faintghost/effectgo/rt v0.0.0
)

replace github.com/danielgtaylor/huma/v2 => ./fakes/huma

replace github.com/gin-gonic/gin => ./fakes/gin

replace github.com/faintghost/effectgo/rt => ../../../../rt
