package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestParseConnectivityEndpointUsesMinimalAliases(t *testing.T) {
	got, err := parseConnectivityEndpointInput("app-b/sql")
	if err != nil {
		t.Fatal(err)
	}
	if got.Application != "app-b" || got.Environment != "" || got.Service != "postgres" || got.Port != 0 {
		t.Fatalf("endpoint=%#v", got)
	}

	got, err = parseConnectivityEndpointInput("app-b@prod/api:8080")
	if err != nil {
		t.Fatal(err)
	}
	if got.Application != "app-b" || got.Environment != "prod" || got.Service != "api" || got.Port != 8080 {
		t.Fatalf("endpoint=%#v", got)
	}
}

func TestFormatConnectivityEndpointKeepsSimpleSQLForm(t *testing.T) {
	got := formatConnectivityEndpoint(application.ConnectivityEndpoint{
		Application: "app-b",
		Environment: "dev",
		Service:     "postgres",
		Port:        5432,
	})
	if got != "app-b@dev/sql" {
		t.Fatalf("formatted endpoint=%q", got)
	}
}

func TestConnectivityServiceAliasMatchesManagedInstances(t *testing.T) {
	if !connectivityServiceMatches("postgres", "postgres-analytics") {
		t.Fatal("sql alias did not match named PostgreSQL instance")
	}
	if !connectivityServiceMatches("valkey", "valkey-sessions") {
		t.Fatal("cache alias did not match named Valkey instance")
	}
	if connectivityServiceMatches("postgres", "api") {
		t.Fatal("sql alias matched unrelated service")
	}
}

func TestConnectivityCommandsAreDiscoverable(t *testing.T) {
	root := rootCommand()
	want := map[string]bool{"connect": false, "disconnect": false, "connections": false}
	for _, child := range root.Children {
		if _, ok := want[child.Name]; ok {
			want[child.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("%s command is missing", name)
		}
	}
}


func TestConnectivityProjectEnvironmentRecognizesCurrentApplicationProject(t *testing.T) {
	tests := []struct {
		name        string
		project     string
		application string
		namespace   string
		wantEnv     string
		wantOK      bool
	}{
		{
			name:        "implicit local target",
			project:     bhruntime.ApplicationProjectName("", "baseharbor-demo", "dev"),
			application: "baseharbor-demo",
			wantEnv:     "dev",
			wantOK:      true,
		},
		{
			name:        "explicit target",
			project:     bhruntime.ApplicationProjectName("demo-docker", "baseharbor-demo", "dev"),
			application: "baseharbor-demo",
			namespace:   "demo-docker",
			wantEnv:     "dev",
			wantOK:      true,
		},
		{
			name:        "legacy fallback",
			project:     "baseharbor-workload-demo-docker-baseharbor-demo-dev",
			application: "baseharbor-demo",
			namespace:   "demo-docker",
			wantEnv:     "dev",
			wantOK:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEnv, gotOK := connectivityProjectEnvironment(tt.project, tt.application, tt.namespace)
			if gotEnv != tt.wantEnv || gotOK != tt.wantOK {
				t.Fatalf("connectivityProjectEnvironment() = (%q, %v), want (%q, %v)", gotEnv, gotOK, tt.wantEnv, tt.wantOK)
			}
		})
	}
}
