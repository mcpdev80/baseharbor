package main

import "testing"

func TestInstallationDestroyAuthorizationEnvironment(t *testing.T) {
	tests := []struct {
		name         string
		environments []string
		want         string
	}{
		{name: "empty installation defaults to dev", want: "dev"},
		{name: "dev-only installation stays dev", environments: []string{"dev"}, want: "dev"},
		{name: "test workload selects test", environments: []string{"dev", "test"}, want: "test"},
		{name: "staging outranks test", environments: []string{"test", "staging"}, want: "staging"},
		{name: "production workload selects prod", environments: []string{"dev", "prod"}, want: "prod"},
		{name: "input order does not change selection", environments: []string{"prod", "dev", "test"}, want: "prod"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := installationDestroyAuthorizationEnvironment(test.environments); got != test.want {
				t.Fatalf("installationDestroyAuthorizationEnvironment(%v) = %q, want %q", test.environments, got, test.want)
			}
		})
	}
}
