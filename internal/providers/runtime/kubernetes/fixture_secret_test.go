package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"testing"
)

// CreateReferenceDemoFixtureSecret is only compiled into this proof's test
// binary. Native fixtures prepare owned Secret data outside the portable plan.
func CreateReferenceDemoFixtureSecret(t *testing.T, p Provider, app, environment, namespace string, values map[string]string) {
	t.Helper()
	createFixtureSecret(t, p, app, environment, namespace, values)
}

func createFixtureSecret(t *testing.T, p Provider, app, environment, namespace string, values map[string]string) {
	t.Helper()
	resource := map[string]any{"apiVersion": "v1", "kind": "Secret", "type": "Opaque", "metadata": map[string]any{"name": app + "-test-bindings", "namespace": namespace, "labels": ownershipLabels(app, environment)}, "stringData": values}
	input, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(context.Background(), p.KubectlPath(), "apply", "-f", "-")
	command.Stdin = bytes.NewReader(input)
	if err := command.Run(); err != nil {
		t.Fatalf("prepare owned proof Secret: %v", err)
	}
}
