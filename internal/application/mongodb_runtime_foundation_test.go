package application

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestMongoDBRuntimeFoundationIsApplicationScopedPersistentAndTLSGated(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "documents",
		Environment:   "dev",
		Services: Services{
			DocumentDatabase:          true,
			DocumentDatabaseInstances: map[string]ServiceInstance{"primary": {}},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	placement, err := ResolveProviderPlacement(m, capability.ProviderMongoDB)
	if err != nil {
		t.Fatal(err)
	}
	if placement.Scope != capability.ScopeApplication || placement.Ownership != capability.OwnershipBaseHarbor {
		t.Fatalf("MongoDB placement = %#v", placement)
	}
	compose, err := RuntimeComposeYAMLForProject(m, "bh-documents")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"mongodb-primary:",
		MongoDBImage,
		"MONGO_INITDB_ROOT_USERNAME",
		"mongodb-primary-data:/data/db",
		"mongodb-primary-access:",
		"MONGODB_PRIMARY_HOST_PORT",
		"name: bh-documents_mongodb-primary-data",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("MongoDB compose missing %q:\n%s", want, compose)
		}
	}
	resources := ExpectedRuntimeResourcesForIdentity(m, "bh-compose", "bh-documents")
	var broker, gateway, volume bool
	for _, resource := range resources {
		switch {
		case resource.Kind == "container" && resource.Name == "bh-compose-mongodb-primary-1":
			broker = true
		case resource.Kind == "container" && resource.Name == "bh-compose-mongodb-primary-access-1":
			gateway = true
		case resource.Kind == "volume" && resource.Name == "bh-documents_mongodb-primary-data":
			volume = true
		}
	}
	if !broker || !gateway || !volume {
		t.Fatalf("MongoDB owned resources = %#v", resources)
	}
}
