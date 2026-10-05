package runtime

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

// Docker exposes map-backed inspection fields, while Podman exposes Go structs.
// Exercise the actual inventory template against both representations.
func TestRuntimeContainerInventoryHealthSupportsDockerAndPodman(t *testing.T) {
	tmpl, err := template.New("inspect").Parse(runtimeContainerInspectTemplate)
	if err != nil {
		t.Fatal(err)
	}
	type nativeHealth struct{ Status string }
	type nativeState struct {
		Running  bool
		Health   *nativeHealth
		Status   string
		ExitCode int
		Error    string
	}
	for _, tc := range []struct {
		name   string
		state  any
		health string
	}{
		{"docker healthy", map[string]any{"Running": true, "Health": map[string]any{"Status": "healthy"}, "Status": "running", "ExitCode": 0, "Error": ""}, "healthy"},
		{"docker no health", map[string]any{"Running": true, "Status": "running", "ExitCode": 0, "Error": ""}, ""},
		{"podman healthy", nativeState{Running: true, Health: &nativeHealth{Status: "healthy"}, Status: "running"}, "healthy"},
		{"podman no health", nativeState{Running: true, Status: "running"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := map[string]any{"Id": "id", "Name": "/server", "Config": map[string]any{"Labels": map[string]string{"com.docker.compose.project": "project", "com.docker.compose.service": "postgres"}}, "State": tc.state}
			var out bytes.Buffer
			if err := tmpl.Execute(&out, data); err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(out.String(), "|")
			if len(parts) != 11 || parts[7] != tc.health || parts[6] != "true" {
				t.Fatalf("invalid inventory: %s", out.String())
			}
		})
	}
}
