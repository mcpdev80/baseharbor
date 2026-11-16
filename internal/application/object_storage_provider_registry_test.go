package application

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestReferenceProviderRegistryReusesSharedSeaweedFS(t *testing.T) {
	registry := capability.NewRegistry()
	alpha := WithObjectStorageBuckets(New("alpha", "prod", false, false, false), "assets")
	beta := WithObjectStorageBuckets(New("beta", "prod", false, false, false), "uploads")
	if err := registerReferenceProviders(&registry, alpha); err != nil {
		t.Fatal(err)
	}
	if err := registerReferenceProviders(&registry, beta); err != nil {
		t.Fatal(err)
	}
	instance, err := registry.Resolve(capability.ProviderSeaweedFS, capability.ScopeShared, alpha.ApplicationID, "")
	if err != nil {
		t.Fatal(err)
	}
	if instance.ID != "seaweedfs/shared" {
		t.Fatalf("instance = %#v", instance)
	}
	count := 0
	for _, binding := range registry.Bindings {
		if binding.ProviderInstanceID == instance.ID && binding.Resource.Kind == capability.ObjectStorageS3 {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("S3 bindings = %#v", registry.Bindings)
	}
}
