package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
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
