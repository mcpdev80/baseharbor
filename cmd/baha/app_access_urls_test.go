package main

import (
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/exposure"
)

func TestApplicationAccessURLsUseObservedDynamicPortsAndPublicRoutes(t *testing.T) {
	state := exposure.State{Routes: []exposure.Route{
		{Visibility: "public", Protocol: "https", ProviderProtocol: "http", PublishedPort: 43373},
		{Visibility: "public", Protocol: "https", PublishedPort: 45123},
		{Visibility: "internal", Protocol: "http", PublishedPort: 3000},
		{Visibility: "public", Protocol: "http", PublishedPort: 0},
	}}
	want := []string{"http://192.168.50.5:43373", "https://192.168.50.5:45123"}
	if got := publicApplicationAccessURLs("https://192.168.50.5:18443", state); !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong application links: %v", got)
	}
	for _, address := range []string{"https://127.0.0.1:18443", "https://0.0.0.0:18443", "https://core.example.test:18443"} {
		if got := publicApplicationAccessURLs(address, state); len(got) != 0 {
			t.Fatalf("invented routable IP from %s", address)
		}
	}
}
