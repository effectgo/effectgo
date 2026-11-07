module example.com/upgrade/backend

go 1.26.0

require github.com/faintghost/effectgo/rt v0.0.0

replace github.com/faintghost/effectgo/rt => ./third_party/effectgo-rt
