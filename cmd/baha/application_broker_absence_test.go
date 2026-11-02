package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type brokerInventoryFixture struct {
	bhruntime.RuntimeProvider
	owned, expected     []bhruntime.ProjectResource
	listErr, inspectErr error
	kind                bhruntime.ProviderKind
}

func (f brokerInventoryFixture) Kind() bhruntime.ProviderKind {
	if f.kind == "" {
		return bhruntime.ProviderDocker
	}
	return f.kind
}

func (f brokerInventoryFixture) ListOwnedProjectResources(context.Context, string) ([]bhruntime.ProjectResource, error) {
	return f.owned, f.listErr
}

func (f brokerInventoryFixture) InspectProjectResources(context.Context, string, []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	return f.expected, f.inspectErr
}

func TestBrokerOwnerUnmaterializedRequiresProtectedEmptyInventory(t *testing.T) {
	manifest := application.New("broker-test", "dev", false, false, true)
	for _, tc := range []struct {
		name    string
		fixture brokerInventoryFixture
		marker  string
		symlink bool
		mode    os.FileMode
		want    bool
	}{
		{name: "verified absence", mode: 0700, want: true},
		{name: "unprotected owner", mode: 0755},
		{name: "partial renderer", mode: 0700, marker: "broker-image"},
		{name: "existing compose", mode: 0700, marker: "broker.compose.yaml"},
		{name: "existing identity", mode: 0700, marker: "bindings/runtime-identity"},
		{name: "symlink bindings", mode: 0700, marker: "bindings", symlink: true},
		{name: "podman unit inventory unverified", mode: 0700, fixture: brokerInventoryFixture{kind: bhruntime.ProviderPodman}},
		{name: "inventory failure", mode: 0700, fixture: brokerInventoryFixture{listErr: errors.New("incomplete")}},
		{name: "foreign collision", mode: 0700, fixture: brokerInventoryFixture{inspectErr: errors.New("foreign owner")}},
		{name: "existing project resource", mode: 0700, fixture: brokerInventoryFixture{owned: []bhruntime.ProjectResource{{Kind: "container", Name: "old"}}}},
		{name: "existing declared resource", mode: 0700, fixture: brokerInventoryFixture{expected: []bhruntime.ProjectResource{{Kind: "volume", Name: "old"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "runtime")
			if err := os.Mkdir(dir, tc.mode); err != nil {
				t.Fatal(err)
			}
			files := application.RuntimeFiles{Dir: dir, Bindings: filepath.Join(dir, "bindings"), Namespace: "ci"}
			if tc.marker != "" {
				path := filepath.Join(dir, tc.marker)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if tc.symlink {
					if err := os.Symlink(t.TempDir(), path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := brokerOwnerUnmaterialized(context.Background(), tc.fixture, manifest, files); got != tc.want {
				t.Fatalf("unmaterialized=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestBrokerOwnerUnmaterializedRejectsSymlinkAncestor(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "actual")
	if err := os.MkdirAll(filepath.Join(actual, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(actual, link); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(link, "runtime")
	files := application.RuntimeFiles{Dir: dir, Bindings: filepath.Join(dir, "bindings"), Namespace: "ci"}
	if brokerOwnerUnmaterialized(context.Background(), brokerInventoryFixture{}, application.New("example", "dev", false, false, true), files) {
		t.Fatal("accepted symlinked owner ancestry")
	}
}
