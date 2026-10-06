package application

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type remoteProbeFixture struct {
	project, service string
	argv             []string
	err              error
}

func (f *remoteProbeFixture) ExecService(_ context.Context, project, service string, argv ...string) (string, error) {
	f.project, f.service, f.argv = project, service, argv
	return "1\n", f.err
}

func TestRemoteBackendProbeKeepsTLSAndContainerCredentialReference(t *testing.T) {
	fixture := &remoteProbeFixture{}
	executor := NewRemoteBackendProbeExecutor(fixture, "baseharbor-owned-dev")
	output, err := executor.ProbeBackend(context.Background(), BackendProbe{Kind: BackendProbeSQLSelectOne, Instance: "default", Database: "owned"})
	if err != nil || output != "1\n" || fixture.project != "baseharbor-owned-dev" || fixture.service != "postgres" || len(fixture.argv) != 3 {
		t.Fatal("remote probe did not preserve selected service", fixture, err)
	}
	for _, required := range []string{"$POSTGRES_PASSWORD", "sslmode=verify-ca", "sslrootcert=/run/baseharbor/tls/ca.pem", "SELECT 1"} {
		if !strings.Contains(fixture.argv[2], required) {
			t.Fatal("remote SQL probe lost its secure contract", required)
		}
	}
	fixture.err = errors.New("selected remote target disconnected")
	if _, err := executor.ProbeBackend(context.Background(), BackendProbe{Kind: BackendProbeSQLSelectOne, Database: "owned"}); err == nil {
		t.Fatal("remote failure fell back to a local provider")
	}
	if _, err := (RuntimeBackendProbeExecutor{}).ProbeBackend(context.Background(), BackendProbe{Kind: BackendProbeSQLSelectOne, Database: "owned"}); err == nil {
		t.Fatal("absent backend executor admitted")
	}
}
