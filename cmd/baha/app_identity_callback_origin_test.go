package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type identityOriginRuntime struct {
	bhruntime.RuntimeProvider
	port int
}

func (r identityOriginRuntime) PreferredLocalHTTPSPort() int { return r.port }

type identityOriginExposure struct{ state exposure.PublishedState }

func (r identityOriginExposure) Apply(context.Context, []exposure.PlannedRoute) (exposure.PublishedState, error) {
	return r.state, nil
}
func (r identityOriginExposure) Reconcile(context.Context, []exposure.PlannedRoute) (exposure.PublishedState, error) {
	return r.state, nil
}
func (identityOriginExposure) Verify(context.Context, exposure.Route) error { return nil }
func (identityOriginExposure) Rollback(context.Context) error               { return nil }

func TestManagedIdentityCallbackOriginsFollowEffectiveTargetGateway(t *testing.T) {
	target := configureTestTarget(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	preferred := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	native := identityOriginRuntime{port: preferred}
	manifest := application.Manifest{Name: "callback-proof", Environment: "dev"}
	identity := &managedIdentityExecution{target: target.Name, manifest: manifest, runtime: native}
	realization := identityOriginExposure{state: exposure.PublishedState{Host: "irrelevant.native.host", Routes: []exposure.Route{{Name: "custom-route", Service: "api", Visibility: "public", Protocol: "https", PublishedPort: 443}}}}
	lifecycle := exposure.NewLifecycle("irrelevant.native.host", "local", realization)
	if err := lifecycle.Provision(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	prepared := &managedExposureExecution{driver: lifecycle}
	host, err := devaccess.ApplicationHost(target.Name, manifest.Name, "api")
	if err != nil {
		t.Fatal(err)
	}
	origins, err := managedIdentityExposureOrigins(identity, prepared)
	if err != nil || !reflect.DeepEqual(origins, []string{fmt.Sprintf("https://%s:%d", host, preferred)}) {
		t.Fatalf("fresh runtime origin=%v error=%v", origins, err)
	}
	files, err := devgateway.FilesFor(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	stored := []byte(`{"version":1,"host_port":28443,"routes":[]}`)
	if err := os.WriteFile(files.State, stored, 0600); err != nil {
		t.Fatal(err)
	}
	origins, err = managedIdentityExposureOrigins(identity, prepared)
	if err != nil || !reflect.DeepEqual(origins, []string{"https://" + host + ":28443"}) {
		t.Fatalf("persisted target origin=%v error=%v", origins, err)
	}
	after, err := os.ReadFile(files.State)
	if err != nil || string(after) != string(stored) {
		t.Fatal("callback resolution rewrote gateway authority")
	}
	// A different Target cannot borrow this gateway's port; its existing owner
	// determines its registered callback even with the same runtime preference.
	other, err := devgateway.FilesFor("another-target")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(other.State), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other.State, []byte(`{"version":1,"host_port":30443,"routes":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	identity.target = "another-target"
	otherHost, err := devaccess.ApplicationHost(identity.target, manifest.Name, "api")
	if err != nil {
		t.Fatal(err)
	}
	origins, err = managedIdentityExposureOrigins(identity, prepared)
	if err != nil || !reflect.DeepEqual(origins, []string{"https://" + otherHost + ":30443"}) {
		t.Fatalf("other target origin=%v error=%v", origins, err)
	}
}
