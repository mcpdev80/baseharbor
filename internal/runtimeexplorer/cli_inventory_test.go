package runtimeexplorer

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

type inventoryDirectRuntime struct {
	kind  runtimecontract.ProviderKind
	calls []string
}

func (f *inventoryDirectRuntime) Kind() runtimecontract.ProviderKind { return f.kind }
func (f *inventoryDirectRuntime) ListRuntimeContainers(context.Context) ([]runtimecontract.RuntimeContainer, error) {
	return nil, nil
}
func (f *inventoryDirectRuntime) DirectOutput(_ context.Context, args ...string) (string, error) {
	call := strings.Join(args, " ")
	f.calls = append(f.calls, call)
	switch {
	case strings.HasPrefix(call, "image ls "):
		return "sha256:abc|example/api|v1\nsha256:def|<none>|<none>\n", nil
	case call == "volume ls --format {{.Name}}":
		return "data-a\ndata-b\n", nil
	case call == "network ls --format {{.Name}}":
		return "net-a\nnet-b\n", nil
	case strings.HasPrefix(call, "network inspect --format "):
		return "network-id-a|net-a\nnetwork-id-b|net-b\n", nil
	case strings.HasPrefix(call, "pod ps "):
		return "pod-id|demo-pod|Running\n", nil
	default:
		return "", fmt.Errorf("unexpected runtime call %q", call)
	}
}
func (f *inventoryDirectRuntime) DirectStream(_ context.Context, args ...string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}


func TestCLIInventoryExposesStableReadOnlyResourceIDs(t *testing.T) {
	runtime := &inventoryDirectRuntime{kind: runtimecontract.ProviderDocker}
	backend, err := NewCLIContainerBackend(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := backend.InventoryResourceKinds(), []ResourceKind{KindImage, KindVolume, KindNetwork}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inventory kinds = %#v, want %#v", got, want)
	}

	images, err := backend.ListInventoryResources(context.Background(), KindImage)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 || images[0].ResourceID != "sha256:abc" || images[0].Name != "example/api:v1" {
		t.Fatalf("image inventory = %#v", images)
	}

	volumes, err := backend.ListInventoryResources(context.Background(), KindVolume)
	if err != nil {
		t.Fatal(err)
	}
	if len(volumes) != 2 || volumes[0].ResourceID != "data-a" {
		t.Fatalf("volume inventory = %#v", volumes)
	}

	networks, err := backend.ListInventoryResources(context.Background(), KindNetwork)
	if err != nil {
		t.Fatal(err)
	}
	if len(networks) != 2 || networks[0].ResourceID != "network-id-a" {
		t.Fatalf("network inventory = %#v", networks)
	}
}

func TestCLIInventoryAdvertisesPodmanPodsOnlyForPodman(t *testing.T) {
	runtime := &inventoryDirectRuntime{kind: runtimecontract.ProviderPodman}
	backend, err := NewCLIContainerBackend(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if !containsResourceKind(backend.InventoryResourceKinds(), KindPod) {
		t.Fatal("podman inventory does not advertise pods")
	}
	pods, err := backend.ListInventoryResources(context.Background(), KindPod)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 || pods[0].ResourceID != "pod-id" || pods[0].State != "Running" {
		t.Fatalf("pod inventory = %#v", pods)
	}
}
