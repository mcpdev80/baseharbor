package identityprovider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type selectedKeycloakBootstrapRuntime struct {
	recordingKeycloakRuntime
	selected []string
	err      error
	verified bool
	starts   int
	t        *testing.T
}

func TestKeycloakBootstrapIncludesGeneratedDatabaseDependencies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(path, []byte(keycloakCompose(application.WithHA(application.New("demo", "dev", false, false, false), true), KeycloakFiles{})), 0600); err != nil {
		t.Fatal(err)
	}
	services, err := keycloakBootstrapServices(path)
	if err != nil {
		t.Fatal(err)
	}
	selected := map[string]bool{}
	for _, name := range services {
		selected[name] = true
	}
	for _, required := range []string{"keycloak-1", "keycloak-access", "keycloak-db-init", "keycloak-db-member-1", "keycloak-db-member-2", "keycloak-db-member-3"} {
		if !selected[required] {
			t.Fatalf("initial phase omitted required dependency %s", required)
		}
	}
	if selected["keycloak-2"] || selected["keycloak-3"] {
		t.Fatal("initial phase permits competing database migrations")
	}
}

func (r *selectedKeycloakBootstrapRuntime) UpProjectFilesSelected(_ context.Context, project, dir string, env map[string]string, services []string, files ...string) error {
	if project != "owned" || len(files) != 1 || files[0] != filepath.Join(dir, "compose.yaml") || env["BASEHARBOR_KEYCLOAK_ADMIN_USER"] != "owner" {
		r.t.Fatal("bootstrap lost its owned source or protected environment")
	}
	r.selected = append([]string(nil), services...)
	return r.err
}

func (r *selectedKeycloakBootstrapRuntime) UpProject(ctx context.Context, project, compose, env string) error {
	if !r.verified {
		r.t.Fatal("additional Keycloak members started before authenticated bootstrap")
	}
	r.starts++
	return r.recordingKeycloakRuntime.UpProject(ctx, project, compose, env)
}

func TestKeycloakBootstrapAuthenticatesBeforeStartingAdditionalMembers(t *testing.T) {
	for _, failure := range []string{"none", "runtime", "authentication", "cancellation"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			files := KeycloakFiles{Dir: dir, Project: "owned", Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env")}
			compose := "services:\n  keycloak-1: {}\n  keycloak-2: {}\n  keycloak-3: {}\n  keycloak-access: {}\n  keycloak-db-init: {}\n"
			if err := os.WriteFile(files.Compose, []byte(compose), 0600); err != nil {
				t.Fatal(err)
			}
			if err := writeProtectedEnv(files.Env, map[string]string{"BASEHARBOR_KEYCLOAK_ADMIN_USER": "owner"}); err != nil {
				t.Fatal(err)
			}
			stop := errors.New("bootstrap rejected")
			runtime := &selectedKeycloakBootstrapRuntime{t: t}
			if failure == "runtime" {
				runtime.err = stop
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			checks := 0
			err := applyKeycloakBootstrap(ctx, runtime, files, func(_ context.Context, selected KeycloakFiles) error {
				checks++
				if selected != files || !reflect.DeepEqual(runtime.selected, []string{"keycloak-1", "keycloak-access", "keycloak-db-init"}) {
					t.Fatal("authentication did not follow the selected bootstrap member")
				}
				if failure == "authentication" {
					return stop
				}
				runtime.verified = true
				if failure == "cancellation" {
					cancel()
				}
				return nil
			})
			if failure == "none" {
				if err != nil || checks != 1 || runtime.starts != 1 {
					t.Fatalf("bootstrap did not continue once: checks=%d calls=%v err=%v", checks, runtime.started, err)
				}
			} else if err == nil || runtime.starts != 0 || (failure == "runtime" && checks != 0) {
				t.Fatalf("failed bootstrap activated additional members: checks=%d calls=%v err=%v", checks, runtime.started, err)
			}
		})
	}
}

func TestKeycloakSingleBootstrapDependencies(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"compose.yaml")
 if err:=os.WriteFile(path,[]byte(keycloakCompose(application.New("demo","prod",false,false,false),KeycloakFiles{})),0600);err!=nil{t.Fatal(err)}
 selected,err:=keycloakBootstrapServices(path)
 if err!=nil{t.Fatal(err)}
 want:=map[string]bool{"keycloak-1":true,"keycloak-access":true,"keycloak-db-tls-init":true,"keycloak-db":true,"keycloak-db-init":true}
 for _,name:=range selected { if !want[name] {t.Errorf("unexpected single boot service %s",name)}; delete(want,name) }
 for missing:=range want {t.Errorf("missing single boot dependency %s",missing)}
}
