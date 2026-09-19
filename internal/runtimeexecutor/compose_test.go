package runtimeexecutor

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func TestComposeYAMLUsesNonRootPreparedStateVolume(t *testing.T) {
	got := composeYAML(
		"baseharbor-runtime:test",
		openbao.RuntimeExecutorMTLSFiles{
			CA:   "/tmp/ca.pem",
			Cert: "/tmp/executor-cert.pem",
			Key:  "/tmp/executor-key.pem",
		},
		"/tmp/s3-admin.env",
	)

	if strings.Contains(got, "state-init:") {
		t.Fatalf("runtime executor compose unexpectedly contains privileged state init service:\n%s", got)
	}
	for _, want := range []string{
		"runtime-resource-state:/var/lib/baseharbor/runtime-resources",
		"read_only: true",
		"cap_drop:",
		"- ALL",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime executor compose missing %q:\n%s", want, got)
		}
	}
}
