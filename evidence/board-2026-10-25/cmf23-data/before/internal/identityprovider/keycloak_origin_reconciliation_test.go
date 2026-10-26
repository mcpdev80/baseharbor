package identityprovider

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

type changedOriginRealization struct {
	calls int
	err   error
}

func (r *changedOriginRealization) Apply(context.Context) (KeycloakInstance, error) {
	r.calls++
	return KeycloakInstance{}, r.err
}
func (*changedOriginRealization) Existing(context.Context) (KeycloakInstance, error) {
	return KeycloakInstance{}, errors.New("unexpected existing observation")
}
func (*changedOriginRealization) Destroy(context.Context) error {
	return errors.New("unexpected destruction")
}

func TestKeycloakRealizedOriginChangeCannotReusePrematureProvisioning(t *testing.T) {
	for _, change := range []string{"new-public-origin", "changed-port", "withdrawn-origin"} {
		t.Run(change, func(t *testing.T) {
			failure := errors.New("provider-reconciliation-required")
			realization := &changedOriginRealization{err: failure}
			driver := NewKeycloakDriverWithRealization(realization, application.WithIdentity(application.New("web", "prod", false, false, false)), application.RuntimeFiles{})
			if change != "new-public-origin" {
				if err := driver.SetApplicationOrigins([]string{"https://app.example:8443"}); err != nil {
					t.Fatal(err)
				}
			}
			driver.provisioned = true
			origins := []string{"https://app.example:9443"}
			if change == "withdrawn-origin" {
				origins = nil
			}
			if err := driver.SetApplicationOrigins(origins); err != nil {
				t.Fatal(err)
			}
			err := driver.Provision(t.Context(), capability.Resource{}, capability.Binding{})
			if !errors.Is(err, failure) || realization.calls != 1 {
				t.Fatal("changed callback origin falsely reused completed provisioning", err)
			}
		})
	}
}

func TestKeycloakUnchangedOrInvalidOriginsPreserveCompletedProvisioning(t *testing.T) {
	realization := &changedOriginRealization{err: errors.New("unexpected mutation")}
	driver := NewKeycloakDriverWithRealization(realization, application.WithIdentity(application.New("web", "prod", false, false, false)), application.RuntimeFiles{})
	if err := driver.SetApplicationOrigins([]string{"https://a.example:8443", "https://b.example:9443"}); err != nil {
		t.Fatal(err)
	}
	driver.provisioned = true
	if err := driver.SetApplicationOrigins([]string{" https://b.example:9443/ ", "https://a.example:8443", "https://a.example:8443"}); err != nil {
		t.Fatal(err)
	}
	if err := driver.Provision(t.Context(), capability.Resource{}, capability.Binding{}); err != nil || realization.calls != 0 {
		t.Fatal("equivalent normalized origins repeated provider mutation", err)
	}
	if err := driver.SetApplicationOrigins([]string{"http://invalid.example"}); err == nil {
		t.Fatal("production accepted insecure application origin")
	}
	if err := driver.Provision(t.Context(), capability.Resource{}, capability.Binding{}); err != nil || realization.calls != 0 {
		t.Fatal("invalid origin changed completed provider authority", err)
	}
}
