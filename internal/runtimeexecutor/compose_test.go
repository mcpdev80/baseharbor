package runtimeexecutor

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func TestComposeYAMLInitializesStateVolumeBeforeExecutor(t *testing.T) {
	got := composeYAML(
		"baseharbor-runtime:test",
		openbao.RuntimeExecutorMTLSFiles{
			CA:   "/tmp/ca.pem",
			Cert: "/tmp/executor-cert.pem",
			Key:  "/tmp/executor-key.pem",
		},
		"/tmp/s3-admin.env",
	)

	for _, want := range []string{
		"state-init:",
		"user: \"0:0\"",
		"network_mode: \"none\"",
		"- CHOWN",
		"condition: service_completed_successfully",
		"chown 65532:65532 /var/lib/baseharbor/runtime-resources",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime executor compose missing %q:\n%s", want, got)
		}
	}
}
