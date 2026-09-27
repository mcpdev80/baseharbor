package identityprovider

import (
	"context"
	"testing"
)

type testKeycloakRuntime struct{ engine string }

func (r testKeycloakRuntime) Engine() string { return r.engine }
func (r testKeycloakRuntime) ConfigProject(context.Context, string, string, string) error { return nil }
func (r testKeycloakRuntime) UpProject(context.Context, string, string, string) error { return nil }
func (r testKeycloakRuntime) DestroyProject(context.Context, string, string, string) error { return nil }

func TestManagedOperatorCanonicalBaseURL(t *testing.T) {
	tests := []struct {
		name   string
		engine string
		want   string
	}{
		{name: "docker", engine: "docker", want: "https://auth.baha.localhost"},
		{name: "podman", engine: "podman", want: "https://auth.baha.localhost:8443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_DEV_DOMAIN", "baha.localhost")
			got, err := managedOperatorCanonicalBaseURL(testKeycloakRuntime{engine: tt.engine}, "demo")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("canonical base URL = %q, want %q", got, tt.want)
			}
		})
	}
}
