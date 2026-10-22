package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestAppEditZeroNeedsGuidedPreviewReachesReadonlyPreview reproduces the
// new-edit-new-zero-no gate failure: the Edit wizard of an application without
// any provider needs must advance from the needs screen into the read-only
// decision preview. A zero pending-change set previously returned the bare
// zero-value plan envelope, appMachinePropose rejected it as a conflict, the
// drive loop swallowed the Step error and the wizard never showed the preview.
func TestAppEditZeroNeedsGuidedPreviewReachesReadonlyPreview(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	target := configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	m := application.New("edit-zero-preview", "dev", false, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	m.Workload = application.WorkloadConfig{Components: []string{"api"}}
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  api:\n    image: alpine:3.22\n    command: [sleep, '300']\n")
	sourcePath := filepath.Join(root, application.RepositoryManifestName)
	mustWriteInitializedAuthoredManifest(t, sourcePath, m)
	registerTestDeployment(t, target, m, root, sourcePath)

	before, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader("enter\nno\ncancel\n")
	var out bytes.Buffer
	result, err := runAppEdit(context.Background(), application.DefaultStore(), []string{m.Name}, input, &out, &out)
	transcript := out.String()
	if !strings.Contains(transcript, "Read-only decision preview (not execution)") {
		t.Fatalf("zero-needs Edit never reached the read-only preview: err=%v result=%+v\n%s", err, result, transcript)
	}
	if !strings.Contains(transcript, "Decision returned to caller") && !errors.Is(err, errAppWizardAbort) {
		t.Fatalf("preview must offer the read-only decision return without applying: err=%v\n%s", err, transcript)
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("No/Cancel edit touched the authored source: %v", err)
	}
}
