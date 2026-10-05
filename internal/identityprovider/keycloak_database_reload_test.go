package identityprovider

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type databaseReloadRuntime struct {
	recordingKeycloakRuntime
	services []string
	failOn   string
	failure  error
}

func (r *databaseReloadRuntime) ExecProject(_ context.Context, _, _, _, service string, _ ...string) (string, error) {
	r.services = append(r.services, service)
	if service == r.failOn {
		return "", r.failure
	}
	return "server signaled", nil
}

func TestKeycloakDatabaseReloadFailsClosed(t *testing.T) {
	files := KeycloakFiles{Project: "identity", Compose: "compose.yaml", Env: "runtime.env"}
	if err := reloadKeycloakDatabaseCertificates(context.Background(), &recordingKeycloakRuntime{}, files); err == nil {
		t.Fatal("database certificate rotation accepted a runtime without reload support")
	}
	failure := errors.New("database reload rejected")
	runtime := &databaseReloadRuntime{failOn: "keycloak-db-member-2", failure: failure}
	if err := reloadKeycloakDatabaseCertificates(context.Background(), runtime, files); !errors.Is(err, failure) {
		t.Fatalf("database reload failure = %v, want original failure", err)
	}
	if !reflect.DeepEqual(runtime.services, []string{"keycloak-db-member-1", "keycloak-db-member-2"}) {
		t.Fatalf("reload continued after a member rejected it: %v", runtime.services)
	}
}
