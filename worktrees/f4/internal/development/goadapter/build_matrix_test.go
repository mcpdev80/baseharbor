package goadapter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
)

// Real ecosystem builds are opt-in because module resolution can use the network.
// The HTTP-only case always builds and requires no third-party modules.
func TestGeneratedGoProjectsCompile(t *testing.T) {
	cases := []struct {
		name         string
		capabilities []capability.Kind
	}{
		{"http-only", nil},
		{"queue", []capability.Kind{capability.MessagingQueue}},
		{"pubsub", []capability.Kind{capability.MessagingPubSub}},
		{"stream", []capability.Kind{capability.MessagingStream}},
		{"document", []capability.Kind{capability.DocumentDatabase}},
		{"durable-kv", []capability.Kind{capability.DurableKeyValue}},
		{"sql-cache-queue", []capability.Kind{capability.SQL, capability.KeyValue, capability.MessagingQueue}},
		{"all-client-imports", []capability.Kind{capability.SQL, capability.KeyValue, capability.DurableKeyValue, capability.DocumentDatabase, capability.MessagingQueue, capability.MessagingPubSub, capability.MessagingStream, capability.ObjectStorageS3, capability.TelemetryOTLP}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.capabilities) > 0 && os.Getenv("BASEHARBOR_GENERATED_GO_BUILD") != "true" {
				t.Skip("enable BASEHARBOR_GENERATED_GO_BUILD=true for real dependency/build validation")
			}
			contract := application.PortableContract{Application: "generated-fixture", Capabilities: []capability.Requirement{{Kind: capability.ExposureHTTP, Name: "web"}}}
			for _, kind := range tc.capabilities {
				contract.Capabilities = append(contract.Capabilities, capability.Requirement{Kind: kind, Name: "default"})
			}
			component := development.Component{ID: "app", Role: "backend", Adapter: AdapterID}
			profile := development.StackProfile{APIVersion: development.StackProfileAPIVersion, Kind: development.StackProfileKind, Metadata: development.ProfileMetadata{Name: "go-fixture"}, Components: []development.Component{component}}
			registry, err := development.NewRegistry(Adapter{})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := development.BuildPlan(contract, profile, registry)
			if err != nil {
				t.Fatal(err)
			}
			files, err := (Adapter{}).Bootstrap(plan, component)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			for _, file := range files {
				if err := os.WriteFile(filepath.Join(root, file.Path), file.Content, os.FileMode(file.Mode)); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			for _, args := range [][]string{{"mod", "tidy"}, {"mod", "verify"}, {"build", "-buildvcs=false", "./..."}} {
				cmd := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), args...)
				cmd.Dir = root
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("generated %s: go %v: %v\n%s", tc.name, args, err, output)
				}
			}
		})
	}
}
