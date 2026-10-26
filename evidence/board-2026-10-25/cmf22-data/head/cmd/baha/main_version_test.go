package main

import "testing"

func TestDefaultRuntimeImageUsesReleaseVersion(t *testing.T) {
	got := defaultRuntimeImage("0.1.0")
	want := "ghcr.io/mcpdev80/baseharbor-runtime:0.1.0"
	if got != want {
		t.Fatalf("defaultRuntimeImage() = %q, want %q", got, want)
	}
}

func TestDefaultRuntimeImageTrimsVPrefix(t *testing.T) {
	got := defaultRuntimeImage("v0.2.3")
	want := "ghcr.io/mcpdev80/baseharbor-runtime:0.2.3"
	if got != want {
		t.Fatalf("defaultRuntimeImage() = %q, want %q", got, want)
	}
}

func TestDefaultRuntimeImageUsesEdgeForDevelopment(t *testing.T) {
	for _, version := range []string{"", "dev", "dev-local", "dev-snapshot", "dev-dirty", "0.2.0-dirty"} {
		t.Run(version, func(t *testing.T) {
			got := defaultRuntimeImage(version)
			want := "ghcr.io/mcpdev80/baseharbor-runtime:edge"
			if got != want {
				t.Fatalf("defaultRuntimeImage(%q) = %q, want %q", version, got, want)
			}
		})
	}
}
