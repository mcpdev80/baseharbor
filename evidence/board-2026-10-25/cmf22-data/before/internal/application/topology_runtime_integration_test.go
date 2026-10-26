package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/availability"
	"github.com/mcpdev80/baseharbor/internal/provideroperation"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestApplicationDataDefaultTopologyRuntimeAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_PROVIDER_TOPOLOGY_ACCEPTANCE") != "1" {
		t.Skip("isolated native topology acceptance is not enabled")
	}
	useApplicationScopedDataProviders(t)
	no, yes := false, true
	for _, scenario := range []struct {
		name       string
		global, ha bool
	}{
		{name: "omitted"}, {name: "false"}, {name: "override-single", global: true}, {name: "override-ha", ha: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
			defer cancel()
			runtime, err := runtimeprovider.Resolve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "data-topology-" + scenario.name}
			m := Manifest{Version: CurrentVersion, ApplicationID: MustNewApplicationID(), Name: "data-topology-ci", Environment: "dev", HA: scenario.global, Services: Services{SQL: true, Cache: true, DocumentDatabase: true, MessagingQueue: true}}
			if scenario.name != "omitted" {
				want := &no
				if scenario.ha {
					want = &yes
				}
				m.Availability = map[string]availability.Override{"sql": {HA: &no}, "cache": {HA: want}, "messaging": {HA: want}, "document_database": {HA: want}}
			}
			issuer := serviceissuer.New(t)
			files, err := EnsureRuntime(ctx, issuer, store, m)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				cleanup, stop := context.WithTimeout(context.Background(), 90*time.Second)
				defer stop()
				if err := runtime.DestroyProject(cleanup, files.Project, files.Compose, files.Env); err != nil {
					t.Errorf("owned destroy: %v", err)
				}
				inventory, err := runtime.ListRuntimeContainers(cleanup)
				if err != nil {
					t.Errorf("post-destroy native inventory: %v", err)
				}
				for _, c := range inventory {
					if c.Project == files.Project {
						t.Errorf("owned container survived destroy: %s", c.Service)
					}
				}
			}()
			if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
				t.Fatal(err)
			}
			if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
				t.Fatal(err)
			}
			op := provideroperation.New(runtime, files.Project, files.Compose, files.Env)
			if err := ReconcileRabbitMQCredentials(ctx, op, m, files); err != nil {
				t.Fatal(err)
			}
			if err := ReconcileMongoDBHA(ctx, op, m, files); err != nil {
				t.Fatal(err)
			}
			verify := func() error {
				for _, check := range []func() error{
					func() error { return VerifyPostgresRuntime(ctx, runtime, m, files) },
					func() error { return VerifyValkeyRuntime(ctx, runtime, m, files) },
					func() error { return VerifyRabbitMQRuntime(ctx, m, files) },
					func() error { return VerifyMongoDBRuntime(ctx, m, files) },
					func() error { return VerifyMongoDBHACluster(ctx, op, m, files) },
					func() error { return VerifyValkeyHACluster(ctx, op, m, files) },
					func() error { return VerifyRabbitMQHACluster(ctx, op, m, files) },
				} {
					if err := check(); err != nil {
						return err
					}
				}
				return nil
			}
			deadline := time.Now().Add(2 * time.Minute)
			for {
				err = verify()
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("native authenticated readiness: %v", err)
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(time.Second):
				}
			}
			observe := func() {
				inventory, err := runtime.ListRuntimeContainers(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var running []string
				for _, c := range inventory {
					if c.Project == files.Project && c.Running {
						running = append(running, c.Service)
					}
				}
				observations, err := providertopology.Observe(files.Compose, running, AvailabilityIntent(m), RuntimeTopologyGroups(m))
				if err != nil {
					t.Fatal(err)
				}
				found := map[string]bool{}
				for _, o := range observations {
					if o.DeclaredMembers == 0 {
						continue
					}
					expected := 1
					if scenario.ha && o.Provider != "postgresql" {
						expected = 3
					}
					if o.Members != expected || o.DataMembers != expected {
						t.Fatalf("%s: native data/service members=%d/%d expected=%d (%s)", o.Provider, o.DataMembers, o.Members, expected, o.Detail())
					}
					found[o.Provider] = true
					t.Log(o.Detail())
				}
				for _, p := range []string{"postgresql", "valkey", "rabbitmq", "mongodb"} {
					if !found[p] {
						t.Fatalf("native provider missing: %s", p)
					}
				}
			}
			observe()
			before, err := os.ReadFile(files.Compose)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := EnsureRuntime(ctx, issuer, store, m); err != nil {
				t.Fatalf("repeated materialization: %v", err)
			}
			after, err := os.ReadFile(files.Compose)
			if err != nil || string(before) != string(after) {
				t.Fatal("repeated up changed topology")
			}
			if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
				t.Fatal(err)
			}
			if err := verify(); err != nil {
				t.Fatalf("repeated up semantic verification: %v", err)
			}
			observe()
			changed := m
			changed.Availability = map[string]availability.Override{"sql": {HA: &no}, "cache": {HA: &yes}, "messaging": {HA: &yes}, "document_database": {HA: &yes}}
			if scenario.ha {
				changed.Availability["cache"] = availability.Override{HA: &no}
			}
			if _, err := EnsureRuntime(ctx, issuer, store, changed); err == nil || !strings.Contains(err.Error(), "topology") {
				t.Fatalf("existing data topology changed: %v", err)
			}
			if err := verify(); err != nil {
				t.Fatalf("rejected transition affected data service: %v", err)
			}
		})
	}
}
