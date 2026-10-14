package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

func TestRegisteredOIDCVerificationRejectsSelectedMaterialChangeAfterDelivery(t *testing.T) {
	for _, change := range []string{"credential", "trust", "missing-credential"} {
		t.Run(change, func(t *testing.T) {
			r, secret, _ := restoreRegisteredIdentityFixture(t, true)
			files := application.RuntimeFilesFor(r.Store, r.Manifest)
			driver, err := identityprovider.NewRegisteredExternalDriver(r.Manifest, files, r.TargetStateRoot)
			if err != nil {
				t.Fatal(err)
			}
			if err := driver.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(files.ApplicationEnv)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "credential":
				err = os.WriteFile(secret, []byte("fixture-private-after-delivery"), 0600)
			case "missing-credential":
				err = os.Remove(secret)
			case "trust":
				reg, lookupErr := application.ResolveExternalProviderRegistrationAt(r.TargetStateRoot, r.Manifest, capability.ProviderExternalOIDC)
				if lookupErr != nil {
					t.Fatal(lookupErr)
				}
				data, readErr := os.ReadFile(reg.Trust.CAReference)
				if readErr != nil {
					t.Fatal(readErr)
				}
				err = os.WriteFile(reg.Trust.CAReference, append(data, data...), 0644)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := driver.Verify(t.Context(), capability.Resource{}, capability.Binding{}); err == nil {
				t.Fatal("changed selected source falsely verified unchanged delivered identity")
			}
			after, err := os.ReadFile(files.ApplicationEnv)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("verification rewrote delivered identity after source drift")
			}
		})
	}
}

