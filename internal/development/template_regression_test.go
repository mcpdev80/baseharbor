package development_test

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/development/goadapter"
	"github.com/mcpdev80/baseharbor/internal/development/nextjsadapter"
	"github.com/mcpdev80/baseharbor/internal/development/pythonadapter"
	"github.com/mcpdev80/baseharbor/internal/development/quarkusadapter"
	"go.yaml.in/yaml/v3"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func templateFixture(t *testing.T, adapter development.Adapter) map[string]string {
	t.Helper()
	manifest := application.Manifest{Version: application.CurrentVersion, Name: "template-probe", Environment: "dev", Services: application.Services{SQL: true, Secrets: true}, Secrets: application.SecretRequirements{Required: []application.SecretRequirement{{Name: "API_TOKEN"}}}, Workload: application.WorkloadConfig{Components: []string{"app"}}, Exposures: []application.HTTPExposureRequirement{{Name: "web", Service: "app", Port: 8080, Protocol: "http", Visibility: "public"}}}
	contract, err := application.PortableContractFromManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	component := development.Component{ID: "app", Role: "backend", Adapter: adapter.Descriptor().ID}
	profile := development.StackProfile{APIVersion: development.StackProfileAPIVersion, Kind: development.StackProfileKind, Metadata: development.ProfileMetadata{Name: "regression"}, Components: []development.Component{component}}
	registry, err := development.NewRegistry(adapter)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := development.BuildPlan(contract, profile, registry)
	if err != nil {
		t.Fatal(err)
	}
	files, err := adapter.Bootstrap(plan, component)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	for _, file := range files {
		result[file.Path] = string(file.Content)
	}
	return result
}

func TestGeneratedTemplatesDeliverContractBindings(t *testing.T) {
	for _, adapter := range []development.Adapter{goadapter.Adapter{}, nextjsadapter.Adapter{}, pythonadapter.Adapter{}, quarkusadapter.Adapter{}} {
		t.Run(adapter.Descriptor().ID, func(t *testing.T) {
			files := templateFixture(t, adapter)
			var compose struct {
				Services map[string]struct {
					Environment map[string]string `yaml:"environment"`
				} `yaml:"services"`
			}
			if err := yaml.Unmarshal([]byte(files["compose.yaml"]), &compose); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"API_TOKEN", "DATABASE_URL"} {
				if compose.Services["app"].Environment[name] != "${"+name+"}" {
					t.Fatalf("%s contract binding does not reach workload environment", name)
				}
			}
			sourceUsesSecret := false
			for _, path := range []string{"main.go", "lib/capabilities.ts", "app.py", "src/main/java/dev/baseharbor/AppResource.java"} {
				if strings.Contains(files[path], "API_TOKEN") {
					sourceUsesSecret = true
				}
			}
			if !sourceUsesSecret {
				t.Fatal("generated source ignores the contract's named secret")
			}
			if dockerfile := files["Dockerfile"]; strings.Contains(dockerfile, "go mod tidy") {
				if strings.Index(dockerfile, "COPY . .") > strings.Index(dockerfile, "go mod tidy") {
					t.Fatal("source overwrites the module state after tidy")
				}
			}
			if source, ok := files["lib/capabilities.ts"]; ok {
				if !strings.Contains(source, "export function runtimeBindings()") || strings.Contains(files["app/page.tsx"], "capabilities") || !strings.Contains(files["app/healthz/route.ts"], "force-dynamic") {
					t.Fatal("runtime binding validation executes during static page build")
				}
			}
			if pom, ok := files["pom.xml"]; ok {
				if !strings.Contains(pom, "<goal>build</goal>") {
					t.Fatal("Quarkus packaging not bound to Maven lifecycle")
				}
			}
		})
	}
}

// Native builds deliberately have no runtime credentials. This gate proves
// fresh buildability, not Core delivery or managed-provider readiness.
func TestGeneratedTemplatesNativeBuild(t *testing.T) {
	if os.Getenv("BASEHARBOR_TEMPLATE_BUILD_ACCEPTANCE") != "1" {
		t.Skip("requires isolated native Docker/Podman build host")
	}
	engine := os.Getenv("BASEHARBOR_TEST_RUNTIME")
	if engine != "docker" && engine != "podman" {
		t.Fatal("explicit build runtime required")
	}
	for _, adapter := range []development.Adapter{goadapter.Adapter{}, nextjsadapter.Adapter{}, pythonadapter.Adapter{}, quarkusadapter.Adapter{}} {
		t.Run(adapter.Descriptor().ID, func(t *testing.T) {
			root := t.TempDir()
			for path, content := range templateFixture(t, adapter) {
				destination := filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(destination, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			image := "localhost/baseharbor-template-probe:" + strings.ReplaceAll(adapter.Descriptor().ID, "/", "-")
			command := exec.CommandContext(ctx, engine, "build", "--pull", "-t", image, root)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("fresh generated image failed: %v\n%s", err, output)
			}
			t.Cleanup(func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if output, err := exec.CommandContext(cleanup, engine, "image", "rm", image).CombinedOutput(); err != nil {
					t.Errorf("image cleanup failed: %v\n%s", err, output)
				}
			})
			t.Logf("fresh %s SQL + managed-secret image builds without runtime values", adapter.Descriptor().ID)
		})
	}
}

// Exercise the full greenfield writer and normal workload resolver, rather than
// only testing adapter output. YAML emitter indentation is not a Compose rule.
func TestCreatedTemplatesResolveManagedWorkload(t *testing.T) {
	for _, adapter := range []development.Adapter{goadapter.Adapter{}, nextjsadapter.Adapter{}, pythonadapter.Adapter{}, quarkusadapter.Adapter{}} {
		t.Run(adapter.Descriptor().ID, func(t *testing.T) {
			registry, err := development.NewRegistry(adapter)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(t.TempDir(), "app")
			created, err := development.CreateApplication(root, development.NewApplicationRequest{Name: "generated-workload", Adapter: adapter.Descriptor().ID, Capabilities: []capability.Kind{capability.SQL, capability.Secrets, capability.ExposureHTTP}, Secrets: []string{"API_TOKEN"}}, registry)
			if err != nil {
				t.Fatal(err)
			}
			services, _, found, err := application.SelectedWorkloadServices(root, created.Manifest)
			if err != nil || !found || len(services) != 1 || services[0] != "app" {
				t.Fatalf("generated application cannot enter normal workload preflight: %v %v", services, err)
			}
		})
	}
}
