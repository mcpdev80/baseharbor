package application

import (
	"os"
	"path/filepath"
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

	root := t.TempDir()
	files := RuntimeFiles{Dir: root, Env: filepath.Join(root, "runtime.env")}
	if err := ensureRuntimeEnv(files.Env, m); err != nil {
		t.Fatal(err)
	}
	if err := ensureMongoDBInitFiles(files, m); err != nil {
		t.Fatal(err)
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	appUser := values[mongodbRuntimeKey("primary", "USER")]
	appPassword := values[mongodbRuntimeKey("primary", "PASSWORD")]
	adminUser := values[mongodbRuntimeKey("primary", "ADMIN_USER")]
	adminPassword := values[mongodbRuntimeKey("primary", "ADMIN_PASSWORD")]
	if appUser == "" || appPassword == "" || adminUser == "" || adminPassword == "" {
		t.Fatal("MongoDB application/admin credentials must all be present")
	}
	if appUser == adminUser || appPassword == adminPassword {
		t.Fatal("MongoDB application credentials must be isolated from provider-admin credentials")
	}
	initPath := filepath.Join(root, "providers", "mongodb", "primary", "init.js")
	info, err := os.Stat(initPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("MongoDB init script permissions = %o, want owner-only", info.Mode().Perm())
	}
	initData, err := os.ReadFile(initPath)
	if err != nil {
		t.Fatal(err)
	}
	initText := string(initData)
	if !strings.Contains(initText, "readWrite") || !strings.Contains(initText, values[mongodbRuntimeKey("primary", "DB")]) {
		t.Fatalf("MongoDB scoped application user is missing from init script:\n%s", initText)
	}
	if strings.Contains(initText, adminPassword) {
		t.Fatal("MongoDB provider-admin password leaked into application-user init script")
	}
}
