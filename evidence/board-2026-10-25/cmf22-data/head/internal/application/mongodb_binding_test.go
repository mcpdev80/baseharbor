package application

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestMongoDBServiceBindingUsesApplicationCredentialsOnly(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "catalog",
		Environment:   "dev",
		Services: Services{
			DocumentDatabase:          true,
			DocumentDatabaseInstances: map[string]ServiceInstance{"documents": {}},
		},
	}
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	files, err := EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	appUser := values[mongodbRuntimeKey("documents", "USER")]
	appPassword := values[mongodbRuntimeKey("documents", "PASSWORD")]
	adminUser := values[mongodbRuntimeKey("documents", "ADMIN_USER")]
	adminPassword := values[mongodbRuntimeKey("documents", "ADMIN_PASSWORD")]

	bindingDir := filepath.Join(files.Bindings, "mongodb", "documents")
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(bindingDir, name))
		if err != nil {
			t.Fatalf("read binding %s: %v", name, err)
		}
		return strings.TrimSpace(string(data))
	}
	if read("username") != appUser || read("password") != appPassword {
		t.Fatal("MongoDB binding does not contain application-scoped credentials")
	}
	if read("username") == adminUser || read("password") == adminPassword {
		t.Fatal("MongoDB provider-admin credential leaked into service binding")
	}
	uri, err := url.Parse(read("uri"))
	if err != nil {
		t.Fatal(err)
	}
	if uri.Scheme != "mongodb" || uri.Query().Get("tls") != "true" {
		t.Fatalf("MongoDB binding URI = %q", uri.String())
	}
	if uri.Query().Get("authSource") != values[mongodbRuntimeKey("documents", "DB")] {
		t.Fatalf("MongoDB binding authSource = %q", uri.Query().Get("authSource"))
	}
	if strings.TrimSpace(read("certificates")) == "" {
		t.Fatal("MongoDB binding trust material is empty")
	}
}
