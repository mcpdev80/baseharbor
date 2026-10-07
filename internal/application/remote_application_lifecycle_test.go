package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go.yaml.in/yaml/v3"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

type applicationLifecycleTransport struct {
	managedRuntimeTransport
	project                   string
	running                   map[string]bool
	units                     map[string]string
	activations               []string
	probeFailure              bool
	workloadHealthUnavailable bool
}

func (f *applicationLifecycleTransport) Dispatch(ctx context.Context, scope targetenrollment.Scope, request targetsession.Request) (targetsession.Response, error) {
	response, err := f.managedRuntimeTransport.Dispatch(ctx, scope, request)
	if err != nil {
		return response, err
	}
	var result any
	switch request.Operation {
	case "runtime.compose.apply":
		var payload struct {
			Services []string `json:"services"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || len(payload.Services) == 0 {
			f.t.Fatal("application phase omitted explicit service selection")
		}
		for _, service := range payload.Services {
			f.running[service] = true
			f.activations = append(f.activations, service)
		}
	case "runtime.quadlet.apply":
		var payload struct {
			Name    string `json:"name"`
			Content string `json:"content"`
			Enable  bool   `json:"enable"`
		}
		_ = json.Unmarshal(request.Payload, &payload)
		for _, line := range strings.Split(payload.Content, "\n") {
			if strings.HasPrefix(line, "Label=com.docker.compose.service=") {
				f.units[payload.Name] = strings.TrimPrefix(line, "Label=com.docker.compose.service=")
			}
		}
		if payload.Enable && f.units[payload.Name] != "" {
			service := f.units[payload.Name]
			f.running[service] = true
			f.activations = append(f.activations, service)
		}
	case "runtime.resource.list":
		var inventory []map[string]string
		for service, running := range f.running {
			if running {
				inventory = append(inventory, map[string]string{"id": lifecycleContainerID(service), "compose_project": f.project, "compose_service": service})
			}
		}
		result = inventory
	case "runtime.resource.inspect":
		var payload struct {
			ID string `json:"resource_id"`
		}
		_ = json.Unmarshal(request.Payload, &payload)
		for service := range f.running {
			health := "healthy"
			if service == "api" && f.workloadHealthUnavailable {
				health = ""
			}
			if lifecycleContainerID(service) == payload.ID {
				result = []any{map[string]any{"Id": payload.ID, "Config": map[string]any{"Labels": map[string]string{"com.docker.compose.project": f.project, "com.docker.compose.service": service}}, "State": map[string]any{"Running": f.running[service], "Status": "running", "Health": map[string]string{"Status": health}}}}
			}
		}
	case "runtime.exec":
		result = map[string]any{"stdout": "1\n", "exit_code": 0}
		if f.probeFailure {
			result = map[string]any{"stdout": "", "exit_code": 1}
		}
	case "runtime.compose.destroy":
		f.running = map[string]bool{}
	case "runtime.quadlet.remove":
		var payload struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(request.Payload, &payload)
		delete(f.running, f.units[payload.Name])
	}
	if result != nil {
		response.Result, _ = json.Marshal(result)
	}
	return response, nil
}

func lifecycleContainerID(service string) string {
	digest := sha256.Sum256([]byte(service))
	return hex.EncodeToString(digest[:])
}

func remoteApplicationFixture(t *testing.T, kind string) (*RemoteManagedRuntime, *applicationLifecycleTransport, Manifest) {
	t.Helper()
	files, manifest := remoteRuntimeProjectionFixture(t)
	repository := t.TempDir()
	compose := filepath.Join(repository, "compose.yaml")
	if err := os.WriteFile(compose, []byte("services:\n  api:\n    image: docker.io/library/alpine:3.23\n    user: '1000:1000'\n    healthcheck: {test: ['CMD', 'true'], interval: 1s}\n    command: ['sh', '-ec', 'echo $$HOME; sleep 300']\n    networks: [baseharbor-backend]\nnetworks:\n  baseharbor-backend:\n    external: true\n    name: "+ApplicationBackendNetworkNameForProject(files.ResourceProject)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	workload := &WorkloadFiles{RepositoryRoot: repository, Compose: compose, Project: files.Project, Services: []string{"api"}}
	scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "selected", NodeID: "selected-node", Runtime: kind}
	transport := &applicationLifecycleTransport{managedRuntimeTransport: managedRuntimeTransport{t: t, scope: scope}, project: files.Project, running: map[string]bool{}, units: map[string]string{}}
	runtime, err := NewRemoteApplicationRuntime(transport, scope, files, manifest, workload, nil)
	if err != nil {
		t.Fatal(err)
	}
	return runtime, transport, manifest
}

func TestRemoteApplicationProviderFirstRepairAndSnapshotTeardown(t *testing.T) {
	for _, kind := range []string{"docker", "podman"} {
		t.Run(kind, func(t *testing.T) {
			runtime, transport, manifest := remoteApplicationFixture(t, kind)
			state := t.TempDir()
			if err := os.Chmod(state, 0700); err != nil {
				t.Fatal(err)
			}
			var record targetsession.ProjectRecord
			if err := runtime.Publish(context.Background(), func(project targetsession.ProjectRecord) error {
				record = project
				return runtime.SaveSnapshot(state, project)
			}); err != nil {
				t.Fatal(err)
			}
			if err := runtime.ApplyApplication(context.Background(), manifest, false); err != nil {
				t.Fatal(err)
			}
			if len(transport.activations) != 2 || !reflect.DeepEqual(transport.activations, []string{"postgres", "api"}) {
				t.Fatal("workload started before verified provider", transport.activations)
			}
			delete(transport.running, "api")
			restored, err := RestoreRemoteManagedSnapshot(transport, transport.scope, state, record)
			if err != nil {
				t.Fatal(err)
			}
			if err := restored.ApplyApplication(context.Background(), manifest, true); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(transport.activations, []string{"postgres", "api", "api"}) {
				t.Fatal("workload repair restarted healthy provider", transport.activations)
			}
			if err := restored.DestroyApplication(context.Background(), manifest, false); err != nil {
				t.Fatal(err)
			}
			if err := restored.RemoveSnapshot(state); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(state, remoteManagedSnapshotFile)); !os.IsNotExist(err) {
				t.Fatal("snapshot remained after verified cleanup", err)
			}
			stages := 0
			for _, call := range transport.calls {
				if call.Operation == "artifact.bundle.stage" {
					stages++
				}
			}
			if stages != 1 {
				t.Fatal("repair or teardown republished mutable source", stages)
			}
		})
	}
}

func TestRemoteApplicationFailedProviderBlocksWorkloadAndChangedIntent(t *testing.T) {
	runtime, transport, manifest := remoteApplicationFixture(t, "docker")
	if err := runtime.Publish(context.Background(), func(record targetsession.ProjectRecord) error { return record.Validate() }); err != nil {
		t.Fatal(err)
	}
	transport.probeFailure = true
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := runtime.ApplyApplication(ctx, manifest, false); err == nil {
		t.Fatal("failed provider qualified Application")
	}
	if !reflect.DeepEqual(transport.activations, []string{"postgres"}) {
		t.Fatal("workload activated after failed provider", transport.activations)
	}
	before := len(transport.calls)
	changed := manifest
	changed.Environment = "test"
	if runtime.ApplyApplication(context.Background(), changed, true) == nil || runtime.DestroyApplication(context.Background(), changed, false) == nil {
		t.Fatal("changed intent substituted retained publication")
	}
	if len(transport.calls) != before {
		t.Fatal("changed intent reached transport")
	}
}

func TestRemoteApplicationProjectionRejectsBuildRootAndForeignResourcesBeforeStaging(t *testing.T) {
	for _, extra := range []string{"    build: .\n", "    user: '0'\n", "    user: '65001'\n", "    privileged: true\n", "    network_mode: host\n", "    volumes: ['/etc/passwd:/data:ro']\n", "    labels: {com.docker.compose.project: foreign}\n", "    labels: [com.docker.compose.project=foreign]\n", "    container_name: foreign\n"} {
		t.Run(strings.TrimSpace(extra), func(t *testing.T) {
			files, manifest := remoteRuntimeProjectionFixture(t)
			repository := t.TempDir()
			compose := filepath.Join(repository, "compose.yaml")
			if err := os.WriteFile(compose, []byte("services:\n  api:\n    image: alpine:3.23\n    user: '1000:1000'\n    healthcheck: {test: ['CMD', 'true'], interval: 1s}\n"+extra), 0600); err != nil {
				t.Fatal(err)
			}
			scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "selected", NodeID: "selected-node", Runtime: "docker"}
			transport := &managedRuntimeTransport{t: t, scope: scope}
			workload := &WorkloadFiles{RepositoryRoot: repository, Compose: compose, Project: files.Project, Services: []string{"api"}}
			if _, err := NewRemoteApplicationRuntime(transport, scope, files, manifest, workload, nil); err == nil {
				t.Fatal("unsafe remote workload admitted")
			}
			if len(transport.calls) != 0 {
				t.Fatal("unsafe workload reached execution node")
			}
		})
	}
}

func TestRemoteApplicationDirectoryBindingsAreConfinedFileMounts(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "bindings")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ca.pem"), []byte("test-ca"), 0644); err != nil {
		t.Fatal(err)
	}
	projection := ManagedRuntimeProjection{}
	definition := map[string]any{"volumes": []any{directory + ":/bindings:ro"}}
	if err := projectRemoteWorkloadMounts(&projection, definition, filepath.Join(root, "core"), root); err != nil {
		t.Fatal(err)
	}
	mounts := definition["volumes"].([]any)
	if len(mounts) != 1 || !strings.HasSuffix(mounts[0].(string), "/ca.pem:/bindings/ca.pem:ro") || len(projection.Files) != 1 {
		t.Fatal("directory was exposed instead of confined file mount", mounts)
	}
	if err := os.Symlink(filepath.Join(root, "unrelated"), filepath.Join(directory, "foreign")); err != nil {
		t.Fatal(err)
	}
	if _, err := readRemoteWorkloadBinding(root, "bindings"); err == nil {
		t.Fatal("symlink binding admitted")
	}
}

func TestRemoteApplicationBackendNetworkUsesManagedDefaultIdentity(t *testing.T) {
	runtime, _, _ := remoteApplicationFixture(t, "docker")
	for _, file := range runtime.source {
		if file.Path != runtime.compose {
			continue
		}
		var document map[string]any
		if err := yaml.Unmarshal(file.Data, &document); err != nil {
			t.Fatal(err)
		}
		networks := document["networks"].(map[string]any)
		if _, exists := networks["baseharbor-backend"]; exists {
			t.Fatal("duplicate native backend network alias")
		}
		service := document["services"].(map[string]any)["api"].(map[string]any)
		if !reflect.DeepEqual(service["networks"], []any{"default"}) {
			t.Fatal("workload disconnected from managed default network", service)
		}
	}
}

func TestRemoteApplicationGeneratedWorkloadBindingsCompileForBothRuntimes(t *testing.T) {
	for _, kind := range []string{"docker", "podman"} {
		t.Run(kind, func(t *testing.T) {
			files, manifest := remoteRuntimeProjectionFixture(t)
			repository := t.TempDir()
			if err := os.WriteFile(filepath.Join(repository, "compose.yaml"), []byte("services:\n  api:\n    image: docker.io/library/alpine:3.23\n    user: '1000:1000'\n    healthcheck: {test: ['CMD', 'true'], interval: 1s}\n    read_only: true\n    command: ['sleep', '300']\n"), 0600); err != nil {
				t.Fatal(err)
			}
			workload, found, err := MaterializeWorkload(repository, manifest, files)
			if err != nil || !found {
				t.Fatal("materialize generated bindings", err)
			}
			environment, err := RuntimeEnvironment(files)
			if err != nil {
				t.Fatal(err)
			}
			scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "selected", NodeID: "selected-node", Runtime: kind}
			transport := &managedRuntimeTransport{t: t, scope: scope}
			runtime, err := NewRemoteApplicationRuntime(transport, scope, files, manifest, &workload, environment)
			if err != nil {
				t.Fatal("compile generated workload", err)
			}
			providers, workloads, err := runtime.ApplicationPhases(manifest)
			if err != nil || len(providers) != 1 || !reflect.DeepEqual(workloads, []string{"api"}) {
				t.Fatal("generated phases lost source", providers, workloads, err)
			}
		})
	}
}

func TestRemoteApplicationWorkloadCannotCopyUnreferencedCoreMaterial(t *testing.T) {
	core := t.TempDir()
	if err := os.WriteFile(filepath.Join(core, "unreferenced-manager.json"), []byte("private installation material"), 0600); err != nil {
		t.Fatal(err)
	}
	projection := ManagedRuntimeProjection{}
	definition := map[string]any{"volumes": []any{filepath.Join(core, "unreferenced-manager.json") + ":/manager:ro"}}
	if projectRemoteWorkloadMounts(&projection, definition, core, filepath.Dir(core)) == nil || len(projection.Files) != 0 {
		t.Fatal("unreferenced Core material entered workload publication")
	}
}

func TestRemoteApplicationRunningWorkloadWithoutHealthCannotQualifyReady(t *testing.T) {
	runtime, transport, manifest := remoteApplicationFixture(t, "docker")
	if err := runtime.Publish(context.Background(), func(targetsession.ProjectRecord) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ApplyApplication(context.Background(), manifest, false); err != nil {
		t.Fatal(err)
	}
	transport.workloadHealthUnavailable = true
	if runtime.VerifyApplication(context.Background(), manifest) == nil {
		t.Fatal("running workload without positive health qualified readiness")
	}
}

func TestRemoteApplicationResourceDefinitionsCannotBypassHostConfinement(t *testing.T) {
	for _, definition := range []map[string]any{
		{"driver_opts": map[string]any{"type": "none", "device": "/etc", "o": "bind"}},
		{"driver": "macvlan"},
		{"ipam": map[string]any{"driver": "foreign"}},
		{"labels": map[string]any{"com.docker.compose.project": "foreign"}},
	} {
		if validateRemoteWorkloadResource("volumes", definition) == nil {
			t.Fatal("foreign resource definition bypassed remote confinement", definition)
		}
	}
	if err := validateRemoteWorkloadResource("volumes", map[string]any{"driver": "local"}); err != nil {
		t.Fatal(err)
	}
}
