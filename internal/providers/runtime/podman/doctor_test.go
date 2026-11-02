package podman

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeDoctorChecksUsesVerifiedPodmanAuthority(t *testing.T) {
	for _, tc := range []struct {
		name     string
		info     string
		engineOK bool
	}{
		{"native rootless", `{"host":{"os":"linux","serviceIsRemote":false,"security":{"rootless":true}}}`, true},
		{"remote engine", `{"host":{"os":"linux","serviceIsRemote":true,"security":{"rootless":true}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			podman := "#!/bin/sh\ncase \"$1\" in\ninfo) printf '%s' '" + tc.info + "' ;;\nversion) exit 0 ;;\n*) exit 1 ;;\nesac\n"
			if err := os.WriteFile(filepath.Join(dir, "podman"), []byte(podman), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "podman-system-generator"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			checks := RuntimeDoctorChecks(context.Background())
			if len(checks) != 2 || checks[0].Name != "podman" || checks[0].OK != tc.engineOK || checks[1].Name != "quadlet" || !checks[1].OK {
				t.Fatalf("unexpected checks: %+v", checks)
			}
		})
	}
}
