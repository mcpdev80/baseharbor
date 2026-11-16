package podman

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPodmanExecProjectAsPlacesUserBeforeContainer(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	command := filepath.Join(bin, "podman")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$EXEC_TEST_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXEC_TEST_LOG", log)
	if _, err := quadletExecAs(context.Background(), command, "keycloak-db-member-1", "postgres", nil, "psql", "-h", "/var/run/postgresql"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if want := "exec --user postgres keycloak-db-member-1 psql -h /var/run/postgresql\n"; string(got) != want {
		t.Fatalf("podman exec arguments = %q, want %q", got, want)
	}
	if strings.Contains(string(got), "keycloak-db-member-1 --user") {
		t.Fatal("Podman interpreted the user option as the executable")
	}
}

func TestPodmanNetworkHasActiveConsumers(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "network in use", err: errors.New("network is being used"), want: true},
		{name: "associated containers", err: errors.New("has associated containers with it"), want: true},
		{name: "active endpoints", err: errors.New("network has active endpoints"), want: true},
		{name: "permission denied", err: errors.New("permission denied"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := podmanNetworkHasActiveConsumers(tc.err); got != tc.want {
				t.Fatalf("podmanNetworkHasActiveConsumers(%v)=%v want %v", tc.err, got, tc.want)
			}
		})
	}
}
