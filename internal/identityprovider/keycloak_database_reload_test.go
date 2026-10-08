package identityprovider

import (
	"context"
	"errors"
	"reflect"
	"path/filepath"
	"testing"
	"time"
)

type databaseReloadRuntime struct {
	recordingKeycloakRuntime
	services          []string
	failOn            string
	failure           error
	cancel            context.CancelFunc
	remainingFailures int
}

func (r *databaseReloadRuntime) ExecProject(_ context.Context, _, _, _, service string, _ ...string) (string, error) {
	r.services = append(r.services, service)
	if service == r.failOn && r.remainingFailures != 0 {
		r.remainingFailures--
		if r.cancel != nil {
			r.cancel()
		}
		return "", r.failure
	}
	return "server signaled", nil
}

func TestKeycloakDatabaseReloadFailsClosed(t *testing.T) {
	files := KeycloakFiles{Project: "identity", Compose: "compose.yaml", Env: filepath.Join(t.TempDir(), "runtime.env")}
	if err := writeProtectedEnv(files.Env, map[string]string{"BASEHARBOR_KEYCLOAK_TOPOLOGY":"ha"}); err != nil { t.Fatal(err) }
	if err := reloadKeycloakDatabaseCertificates(context.Background(), &recordingKeycloakRuntime{}, files); err == nil {
		t.Fatal("database certificate rotation accepted a runtime without reload support")
	}
	failure := errors.New("database reload rejected")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &databaseReloadRuntime{failOn: "keycloak-db-member-2", failure: failure, cancel: cancel, remainingFailures: -1}
	if err := reloadKeycloakDatabaseCertificates(ctx, runtime, files); !errors.Is(err, failure) || !errors.Is(err, context.Canceled) {
		t.Fatalf("database reload failure = %v, want original failure", err)
	}
	if !reflect.DeepEqual(runtime.services, []string{"keycloak-db-member-1", "keycloak-db-member-2"}) {
		t.Fatalf("reload continued after a member rejected it: %v", runtime.services)
	}
}

func TestKeycloakDatabaseReloadWaitsForRecoveringMembers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runtime := &databaseReloadRuntime{failOn: "keycloak-db-member-1", failure: errors.New("database system is starting up"), remainingFailures: 1}
	env := filepath.Join(t.TempDir(),"runtime.env")
	if err := writeProtectedEnv(env,map[string]string{"BASEHARBOR_KEYCLOAK_TOPOLOGY":"ha"});err!=nil{t.Fatal(err)}
	if err := reloadKeycloakDatabaseCertificates(ctx, runtime, KeycloakFiles{Env:env}); err != nil {
		t.Fatal(err)
	}
	want := []string{"keycloak-db-member-1", "keycloak-db-member-1", "keycloak-db-member-2", "keycloak-db-member-3"}
	if !reflect.DeepEqual(runtime.services, want) {
		t.Fatalf("database reload did not wait for every member: got %v want %v", runtime.services, want)
	}
}

func TestKeycloakSingleDatabaseReload(t *testing.T) {
 env:=filepath.Join(t.TempDir(),"runtime.env")
 if err:=writeProtectedEnv(env,map[string]string{"BASEHARBOR_KEYCLOAK_TOPOLOGY":"single"});err!=nil{t.Fatal(err)}
 runtime:=&databaseReloadRuntime{}
 if err:=reloadKeycloakDatabaseCertificates(context.Background(),runtime,KeycloakFiles{Env:env});err!=nil{t.Fatal(err)}
 if !reflect.DeepEqual(runtime.services,[]string{"keycloak-db"}) {t.Fatalf("single topology attempted HA reload: %v",runtime.services)}
}
