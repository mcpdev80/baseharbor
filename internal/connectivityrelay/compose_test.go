package connectivityrelay

import (
	"os"
	"strings"
	"testing"
)

func TestEnsureFilesCreatesHardenedDirectedRelay(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "baseharbor-runtime:test")

	files, err := EnsureFiles(RuntimeSpec{
		ID:            "abc123",
		SourceNetwork: "baseharbor-link-abc123",
		SourceAlias:   "app-b-sql",
		TargetNetwork: "baseharbor-app-b-dev_default",
		TargetHost:    "postgres",
		TargetPort:    5432,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"BASEHARBOR_CONNECTIVITY_RELAY_MODE",
		"BASEHARBOR_RELAY_LISTEN_ADDR",
		"BASEHARBOR_RELAY_TARGET_ADDR",
		"app-b-sql",
		"baseharbor-link-abc123",
		"baseharbor-app-b-dev_default",
		"read_only: true",
		"cap_drop:",
		"- ALL",
		"no-new-privileges:true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("relay compose missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"ports:", "/var/run/docker.sock", "privileged: true"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("relay compose unexpectedly contains %q:\n%s", forbidden, text)
		}
	}
}

func TestEnsureFilesRejectsInvalidTargetPort(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	_, err := EnsureFiles(RuntimeSpec{
		ID:            "abc123",
		SourceNetwork: "source",
		SourceAlias:   "target",
		TargetNetwork: "target",
		TargetHost:    "postgres",
		TargetPort:    0,
	})
	if err == nil {
		t.Fatal("expected invalid target port rejection")
	}
}


func TestEnsureFilesRejectsUnsafeRelayID(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	_, err := EnsureFiles(RuntimeSpec{
		ID:            "../../escape",
		SourceNetwork: "source",
		SourceAlias:   "target",
		TargetNetwork: "target",
		TargetHost:    "postgres",
		TargetPort:    5432,
	})
	if err == nil {
		t.Fatal("expected unsafe relay id rejection")
	}
}
