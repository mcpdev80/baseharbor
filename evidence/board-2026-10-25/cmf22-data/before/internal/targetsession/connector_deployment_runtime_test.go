package targetsession

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func qualifyConnectorDeployment(t *testing.T, engine, image, name, quadletRoot string, run func(...string) string, dispatch, rawDispatch func(string, any) Response) {
	t.Helper()
	deployed := name + "-deployed"
	var artifactName, content string
	if engine == "docker" {
		artifactName = "compose.yaml"
		content = "name: " + name + "\nservices:\n  workload:\n    image: " + image + "\n    container_name: " + deployed + "\n    user: '1000:1000'\n    command: ['sleep','300']\n    labels:\n      baseharbor.transport-qualification: 'true'\n"
	} else {
		artifactName = deployed + ".container"
		content = "[Unit]\nDescription=BaseHarbor transport qualification\n[Container]\nImage=" + image + "\nContainerName=" + deployed + "\nUser=1000:1000\nExec=sleep 300\nLabel=baseharbor.transport-qualification=true\n[Service]\nTimeoutStartSec=45\n[Install]\nWantedBy=default.target\n"
	}
	data := []byte(content)
	sum := sha256.Sum256(data)
	response := dispatch("artifact.bundle.stage", map[string]any{"bundle_id": name, "files": []map[string]any{{"path": artifactName, "sha256": hex.EncodeToString(sum[:]), "data": data, "mode": 0600}}})
	var staged struct {
		BundleID string `json:"bundle_id"`
		Files    []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if json.Unmarshal(response.Result, &staged) != nil || staged.BundleID != name || len(staged.Files) != 1 || staged.Files[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("actual staged bundle result differs")
	}
	if rawDispatch("artifact.bundle.stage", map[string]any{"bundle_id": name, "files": []map[string]any{{"path": artifactName, "sha256": hex.EncodeToString(sum[:]), "data": data}}}).Success {
		t.Fatal("existing immutable bundle was replaced")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if engine == "podman" {
			_ = exec.CommandContext(cleanup, "systemctl", "--user", "stop", deployed+".service").Run()
			_ = os.Remove(filepath.Join(quadletRoot, artifactName))
			_ = os.Remove(filepath.Join(quadletRoot, artifactName+".d", "99-baseharbor-activation.conf"))
			_ = exec.CommandContext(cleanup, "systemctl", "--user", "daemon-reload").Run()
		}
		// Exact fixture-only emergency cleanup never proves successful transport
		// destroy. The normal path asserts encrypted destroy before returning.
		_ = exec.CommandContext(cleanup, engine, "rm", "-f", deployed).Run()
		if engine == "docker" {
			_ = exec.CommandContext(cleanup, engine, "network", "rm", name+"_default").Run()
		}
	})
	if engine == "docker" {
		payload := map[string]any{"project_directory": filepath.Dir(staged.Files[0].Path), "files": []string{staged.Files[0].Path}, "timeout_seconds": 45}
		dispatch("runtime.compose.apply", payload)
		if run("inspect", "--format", "{{.State.Running}}", deployed) != "true" {
			t.Fatal("actual remote Compose application is not running")
		}
		dispatch("runtime.compose.apply", payload)
		dispatch("runtime.compose.destroy", payload)
	} else {
		foreignName := name + "-foreign.container"
		foreignPath := filepath.Join(quadletRoot, foreignName)
		foreignData := []byte("[Container]\nImage=" + image + "\nExec=sleep 300\n")
		if err := os.WriteFile(foreignPath, foreignData, 0600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(foreignPath) })
		if rawDispatch("runtime.quadlet.apply", map[string]any{"name": foreignName, "content": content, "enable": true}).Success {
			t.Fatal("foreign Quadlet overwritten through transport")
		}
		if data, err := os.ReadFile(foreignPath); err != nil || string(data) != string(foreignData) {
			t.Fatal("foreign Quadlet changed")
		}
		payload := map[string]any{"name": artifactName, "content": content, "enable": true}
		dispatch("runtime.quadlet.apply", payload)
		if run("inspect", "--format", "{{.State.Running}}", deployed) != "true" {
			t.Fatal("actual remote Quadlet application is not running")
		}
		dispatch("runtime.quadlet.apply", payload)
		dispatch("runtime.quadlet.disable", map[string]any{"name": artifactName})
		dispatch("runtime.quadlet.enable", map[string]any{"name": artifactName})
		dispatch("runtime.quadlet.remove", map[string]any{"name": artifactName})
	}
	if output := run("ps", "-aq", "--filter", "name=^"+deployed+"$"); strings.TrimSpace(output) != "" {
		t.Fatal("encrypted destroy left deployed application containers")
	}
	if run("inspect", "--format", "{{.State.Running}}", name) != "true" {
		t.Fatal("application destroy damaged foreign container")
	}
	t.Log("Actual immutable bundle plus Docker Compose/rootless Podman Quadlet apply, status, repair and destroy passed; test authority only.")
}
