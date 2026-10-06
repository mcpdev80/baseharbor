package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type sharedValkeyRegistrationRuntime struct {
	bhruntime.RuntimeProvider
	ready         map[string]bool
	durableWrites int
}

func (r *sharedValkeyRegistrationRuntime) ConfigProject(context.Context, string, string, string) error {
	return nil
}

func (r *sharedValkeyRegistrationRuntime) UpProject(context.Context, string, string, string) error {
	return nil
}

func (r *sharedValkeyRegistrationRuntime) ExecProject(_ context.Context, _, _, _, service string, _ ...string) (string, error) {
	if !strings.HasPrefix(service, "shared-valkey-") {
		return "", fmt.Errorf("unexpected service %s", service)
	}
	r.ready[service] = true
	return "PONG", nil
}

func (r *sharedValkeyRegistrationRuntime) ExecProjectInput(_ context.Context, _, _, _ string, _ []byte, _ string, args ...string) (string, error) {
	if strings.Contains(strings.Join(args, " "), "__baseharbor_verify__") {
		r.durableWrites++
		return "durable", nil
	}
	return "PONG", nil
}

func TestSharedValkeyRegistersCacheAndDurableInstances(t *testing.T) {
	for _, tc := range []struct {
		name           string
		cache, durable bool
	}{
		{"cache", true, false},
		{"durable", false, true},
		{"combined", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(ProviderScopeEnv(capability.ProviderValkey), "shared")
			m := New("shared-registration", "dev", false, tc.cache, false)
			m.Services.SQL = false
			if tc.durable {
				m = WithKeyValueInstances(m, "store")
			}
			if err := m.Validate(); err != nil {
				t.Fatal(err)
			}
			if !UsesSharedValkey(m) {
				t.Error("declared Valkey instances were excluded from shared placement")
			}
			root := t.TempDir()
			store := Store{Root: filepath.Join(root, "apps"), Namespace: "registration"}
			if _, err := store.Create(m); err != nil {
				t.Fatal(err)
			}
			issuer := serviceissuer.New(t)
			files, err := EnsureRuntime(context.Background(), issuer, store, m)
			if err != nil {
				t.Fatal(err)
			}
			dataDir := filepath.Join(root, "target")
			runtime := &sharedValkeyRegistrationRuntime{ready: map[string]bool{}}
			changed, err := ReconcileSharedBackends(context.Background(), runtime, issuer, dataDir, store.Namespace, m, files)
			if err != nil {
				t.Fatal(err)
			}
			if !changed {
				t.Fatal("shared registration was not reconciled")
			}
			shared := SharedBackendFilesAt(dataDir, store.Namespace, m.Environment)
			state, err := loadSharedBackendState(shared.State, m.Environment)
			if err != nil {
				t.Fatal(err)
			}
			app := state.Applications[sharedBackendApplicationKey(m)]
			want := ValkeyInstanceNames(m)
			if len(app.Cache) != len(want) {
				t.Fatalf("registered=%d want=%d", len(app.Cache), len(want))
			}
			values, err := readRuntimeEnv(files.Env)
			if err != nil {
				t.Fatal(err)
			}
			passwords := map[string]bool{}
			for _, instance := range want {
				resource, ok := app.Cache[instance]
				if !ok {
					t.Fatalf("instance %s was not registered", instance)
				}
				password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
				if err != nil {
					t.Fatal(err)
				}
				if password == "" || passwords[password] {
					t.Fatal("instance credential was missing or reused")
				}
				passwords[password] = true
				if values[valkeyRuntimeKey(instance, "PASSWORD")] != password {
					t.Fatalf("binding credential differs for %s", instance)
				}
				if values[valkeyContainerHostKey(instance)] != sharedValkeyAccessAlias(m, instance) {
					t.Fatalf("binding endpoint differs for %s", instance)
				}
				if _, err := os.Stat(values[valkeyTLSCAKey(instance)]); err != nil {
					t.Fatalf("TLS projection for %s: %v", instance, err)
				}
				if !runtime.ready[sharedValkeyService(m, instance)] {
					t.Fatalf("readiness not verified for %s", instance)
				}
			}
			if err := VerifySharedValkey(context.Background(), runtime, dataDir, store.Namespace, m); err != nil {
				t.Fatal(err)
			}
			wantWrites := 0
			if tc.durable {
				wantWrites = 1
			}
			if runtime.durableWrites != wantWrites {
				t.Fatalf("durable write/read probes=%d want=%d", runtime.durableWrites, wantWrites)
			}
		})
	}
}
