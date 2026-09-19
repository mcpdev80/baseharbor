package application

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestObjectStorageManifestRoundTrip(t *testing.T) {
	m := WithObjectStorageBuckets(Manifest{
		Version:     CurrentVersion,
		Name:        "demo",
		Environment: "dev",
	}, "assets", "backups")

	text := m.YAML()
	if !strings.Contains(text, "object_storage:") || !strings.Contains(text, "assets: {}") || !strings.Contains(text, "backups: {}") {
		t.Fatalf("manifest YAML missing object-storage buckets:\n%s", text)
	}
	parsed, err := ParseYAML(text)
	if err != nil {
		t.Fatal(err)
	}
	got := ObjectStorageBucketNames(parsed)
	if strings.Join(got, ",") != "assets,backups" {
		t.Fatalf("buckets = %v", got)
	}
}

func TestObjectStorageCapabilityBindingUsesSecureReferences(t *testing.T) {
	m := WithObjectStorageBuckets(Manifest{
		Version:     CurrentVersion,
		Name:        "demo",
		Environment: "prod",
	}, "assets")

	bindings, err := CapabilityBindings(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 {
		t.Fatalf("bindings = %#v", bindings)
	}
	binding := bindings[0]
	if binding.Resource.Kind != capability.ObjectStorageS3 || binding.Resource.Provider != capability.ProviderSeaweedFS {
		t.Fatalf("resource = %#v", binding.Resource)
	}
	if binding.ObjectStorageS3 == nil || binding.ObjectStorageS3.Bucket != "assets" {
		t.Fatalf("S3 binding = %#v", binding.ObjectStorageS3)
	}
	if binding.Security == nil {
		t.Fatal("secure binding missing")
	}
	if err := binding.Security.Validate(); err != nil {
		t.Fatalf("secure binding invalid: %v", err)
	}
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"seaweedfs-data", "secret_access_key=", "AWS_SECRET_ACCESS_KEY="} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("provider/secret detail leaked into capability binding: %s", data)
		}
	}
}

func TestObjectStorageSecureBindingsAreBucketScoped(t *testing.T) {
	m := Manifest{Version: CurrentVersion, Name: "demo", Environment: "prod"}
	a := ObjectStorageSecureBinding(m, "assets")
	b := ObjectStorageSecureBinding(m, "backups")
	if a.Credentials[0].Reference == b.Credentials[0].Reference ||
		a.Credentials[1].Reference == b.Credentials[1].Reference {
		t.Fatal("distinct buckets share credential references")
	}
	if len(a.Authorization) != 1 || a.Authorization[0].Audience != "object-storage.s3" {
		t.Fatalf("authorization = %#v", a.Authorization)
	}
}
