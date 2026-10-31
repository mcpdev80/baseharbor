package main

import (
	"testing"
)

func TestEngineBinaryNameSelectsPodmanForPodmanProvider(t *testing.T) {
	for input, want := range map[string]string{
		"":        "docker",
		"docker":  "docker",
		"podman":  "podman",
		"unknown": "docker",
	} {
		got := engineBinaryName(input)
		if got != want {
			t.Errorf("engineBinaryName(%q) = %q, want %q", input, got, want)
		}
	}
}
