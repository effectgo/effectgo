# Shared limits for local and CI checks; package concurrency also bounds compiler memory.
export GOMAXPROCS="${GOMAXPROCS:-2}"
export GOMEMLIMIT="${GOMEMLIMIT:-2GiB}"
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-p=1"
export GOWORK=off
