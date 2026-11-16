package controlplaneruntime

import (
	"os"
	"strings"
	"testing"
)

// The authenticated tenant lookup belongs on operatorMux, not the
// enrollment bootstrap bypass. Losing this registration makes node add 404
// although the Core handler implements the route.
func TestConnectorOwnerContextRegisteredOnAuthenticatedMux(t *testing.T) {
	source, err := os.ReadFile("server_run.go")
	if err != nil {
		t.Fatal(err)
	}
	got := string(source)
	if !strings.Contains(got, "operatorMux.Handle(targetenrollment.OwnerContextPath, handler)") {
		t.Fatal("verified tenant lookup must be mounted behind operator OIDC middleware")
	}
	if strings.Contains(got, "mux.Handle(targetenrollment.OwnerContextPath, handler)") &&
		!strings.Contains(got, "operatorMux.Handle(targetenrollment.OwnerContextPath, handler)") {
		t.Fatal("tenant lookup must not bypass operator authentication")
	}
}
