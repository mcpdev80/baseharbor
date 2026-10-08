package main

import "testing"

func TestResolveTargetWizardChoice(t *testing.T) {
	names := []string{"docker-dev", "local", "podman-test"}
	tests := []struct {
		input, want string
		valid       bool
	}{
		{"1", "docker-dev", true},
		{"3", "podman-test", true},
		{"local", "local", true},
		{"docker-dev", "docker-dev", true},
		{"0", "", false},
		{"4", "", false},
		{"missing", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, valid := resolveTargetWizardChoice(names, tt.input)
		if got != tt.want || valid != tt.valid {
			t.Errorf("choice %q -> %q (%t), want %q (%t)", tt.input, got, valid, tt.want, tt.valid)
		}
	}
}
