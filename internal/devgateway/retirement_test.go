package devgateway

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type gatewayRetirementRuntime struct {
	resources           []bhruntime.ProjectResource
	inspectErr, downErr error
	downCalls           int
}

func (*gatewayRetirementRuntime) ConfigProject(context.Context, string, string, string) error {
	return nil
}
func (*gatewayRetirementRuntime) UpProject(context.Context, string, string, string) error { return nil }
func (*gatewayRetirementRuntime) DestroyProject(context.Context, string, string, string) error {
	return errors.New("data destruction must not be used for last-route retirement")
}
func (r *gatewayRetirementRuntime) InspectProjectResources(context.Context, string, []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	return r.resources, r.inspectErr
}
func (r *gatewayRetirementRuntime) DownProject(context.Context, string, string, string) error {
	r.downCalls++
	if r.downErr == nil {
		r.resources = nil
	}
	return r.downErr
}

func TestLastGatewayRouteRetirementPreservesMaterialsAndRetryOwner(t *testing.T) {
	for _, mode := range []string{"success", "interrupted", "foreign", "modified"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			files := Files{Dir: dir, State: filepath.Join(dir, "routes.json"), Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"), Caddyfile: filepath.Join(dir, "Caddyfile"), Project: "isolated-gateway"}
			previous := state{Version: stateVersion, HostPort: 18443, Routes: normalizedRoutes([]Route{{Owner: "app/demo/dev", Key: "demo", Host: "demo.localhost", Upstream: "http://api:8080", Network: "owned-app"}})}
			compose := renderCompose(files, previous.Routes, nil, previous.HostPort)
			if mode == "modified" {
				compose += "# changed\n"
			}
			paths := map[string]string{files.Compose: compose, files.Env: "BASEHARBOR_GATEWAY_UID=1\nBASEHARBOR_GATEWAY_GID=1\n", files.Caddyfile: "protected retained configuration", filepath.Join(dir, "server-key.pem"): "test-only private material"}
			for path, value := range paths {
				if err := os.WriteFile(path, []byte(value), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := saveState(files.State, previous); err != nil {
				t.Fatal(err)
			}
			runtime := &gatewayRetirementRuntime{resources: []bhruntime.ProjectResource{{Kind: "container", Name: files.Project + "-dev-gateway-1"}}}
			if mode == "interrupted" {
				runtime.downErr = errors.New("interrupted")
			}
			if mode == "foreign" {
				runtime.inspectErr = bhruntime.ErrResourceOwnership
			}
			err := saveRouteStateForReconcile(context.Background(), runtime, files, previous, state{Version: stateVersion, HostPort: previous.HostPort, Routes: []Route{}})
			after, loadErr := loadState(files.State)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if mode == "success" {
				if err != nil || len(after.Routes) != 0 || runtime.downCalls != 1 {
					t.Fatalf("retirement: %#v calls=%d err=%v", after, runtime.downCalls, err)
				}
				if err := verifyEmptyGatewayInactive(context.Background(), runtime, files); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil || !reflect.DeepEqual(previous, after) {
					t.Fatalf("failed retirement lost retry owner: %#v %v", after, err)
				}
				if mode != "interrupted" && runtime.downCalls != 0 {
					t.Fatal("unsafe mutation")
				}
			}
			for path, value := range paths {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != value {
					t.Fatalf("retained material changed: %s %v", path, err)
				}
			}
		})
	}
}
