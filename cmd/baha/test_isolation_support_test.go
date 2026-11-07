package main

import (
	"testing"
)

// init binds the TEST-SCOPE provenance gate of the owner loan (card
// t-20261106-r27-remote-up-podman-owner-missing, lane w286). The hook is a
// package-private var that stays nil in a production binary, so no operator
// environment can activate the loan; testing.Testing is the standard-library
// proof that this code runs inside a `go test` process.
func init() {
	bahaTestProcessEnv = testing.Testing
}
