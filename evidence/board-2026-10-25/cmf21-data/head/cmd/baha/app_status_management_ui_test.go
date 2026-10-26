package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
)

func TestCanonicalDevelopmentManagementSurfacesNewProviderFamilies(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv(application.ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))

	resolved := resolvedApplication{
		Target: deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"},
		Manifest: application.Manifest{
			Version:     application.CurrentVersion,
			Name:        "demo",
			Environment: "dev",
			Services: application.Services{
				KeyValue:                     true,
				KeyValueManagementUI:         true,
				MessagingQueue:               true,
				MessagingManagementUI:        true,
				DocumentDatabase:             true,
				DocumentDatabaseManagementUI: true,
			},
		},
	}

	surfaces := []application.ManagementUISurface{
		{Service: "key-value", URL: "https://127.0.0.1:10001/"},
		{Service: "rabbitmq", URL: "https://127.0.0.1:10002/"},
		{Service: "rabbitmq-events", URL: "https://127.0.0.1:10003/"},
		{Service: "mongodb", URL: "https://127.0.0.1:10004/"},
		{Service: "mongodb-archive", URL: "https://127.0.0.1:10005/"},
	}
	got := canonicalDevelopmentManagementSurfaces(resolved, surfaces)

	wantServices := map[string]string{
		"key-value":       "cache",
		"rabbitmq":        "rabbitmq",
		"rabbitmq-events": "rabbitmq-events",
		"mongodb":         "mongodb",
		"mongodb-archive": "mongodb-archive",
	}
	for _, surface := range got {
		service, ok := wantServices[surface.Service]
		if !ok {
			t.Fatalf("unexpected management surface %#v", surface)
		}
		host, err := devaccess.ApplicationHost("local", "demo", service)
		if err != nil {
			t.Fatal(err)
		}
		want := devgateway.URLForTarget("local", host)
		if surface.URL != want {
			t.Fatalf("%s management URL = %q, want %q", surface.Service, surface.URL, want)
		}
	}
}
