package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestAppNewWizardCancelLeavesNoProject(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())

	oldInput := appNewInput
	defer func() { appNewInput = oldInput }()
	appNewInput = strings.NewReader(strings.Join([]string{
		"cancel-app",
		parent,
		"1",
		"1",
		"n",
		"",
	}, "\n"))

	var out bytes.Buffer
	if err := runAppNewWizard(context.Background(), &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parent, "cancel-app")); !os.IsNotExist(err) {
		t.Fatalf("cancelled wizard mutated project root: %v", err)
	}
	if !strings.Contains(out.String(), "No changes were made.") {
		t.Fatalf("missing cancellation confirmation:\n%s", out.String())
	}
}

func TestAppNewWizardCreatesValidatedProject(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())

	oldInput := appNewInput
	defer func() { appNewInput = oldInput }()
	appNewInput = strings.NewReader(strings.Join([]string{
		"guided-app",
		parent,
		"1",
		"1,2",
		"",
		"",
	}, "\n"))

	var out bytes.Buffer
	if err := runAppNewWizard(context.Background(), &out, &out); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "guided-app")
	for _, path := range []string{
		"baseharbor.yaml",
		"compose.yaml",
		".baseharbor/stack-profile.yaml",
		".baseharbor/development-plan.json",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("expected %s: %v\n%s", path, err, out.String())
		}
	}
	if !strings.Contains(out.String(), "Validation: SATISFIED") {
		t.Fatalf("wizard did not report validated result:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Project root:") {
		t.Fatalf("wizard did not preview project root:\n%s", out.String())
	}
}

func TestAppNewWizardCreateOnlyPreservesPreviewIdentity(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())
	old := appNewInput
	defer func() { appNewInput = old }()
	appNewInput = strings.NewReader(strings.Join([]string{
		"identity-preserved", parent, "1", "1", "", "",
	}, "\n"))
	var output bytes.Buffer
	if err := runAppNewWizard(context.Background(), &output, &output); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "identity-preserved")
	manifest, err := os.ReadFile(filepath.Join(root, "baseharbor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "name: identity-preserved") {
		t.Fatalf("created contract missing application name: %s", manifest)
	}
	if !strings.Contains(output.String(), "Next: cd ") || !strings.Contains(output.String(), "baha up") {
		t.Fatal("create-only must advise explicit up, not start runtime")
	}
}

func TestAppNewWizardEOFBeforeConsentWritesNothing(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())
	old := appNewInput
	defer func() { appNewInput = old }()
	appNewInput = strings.NewReader(strings.Join([]string{
		"cancel-at-confirm", parent, "1", "1",
	}, "\n"))
	var output bytes.Buffer
	_ = runAppNewWizard(context.Background(), &output, &output)
	if _, err := os.Stat(filepath.Join(parent, "cancel-at-confirm")); !os.IsNotExist(err) {
		t.Fatalf("EOF before consent wrote project: %v", err)
	}
}

func TestStackCreateWizardBuildsReusableProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())

	registry, err := referenceDevelopmentRegistry()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := development.LoadProfileCatalog(".", builtinDevelopmentProfiles(registry))
	if err != nil {
		t.Fatal(err)
	}

	input := strings.NewReader(strings.Join([]string{
		"team-stack",
		"1",
		"go",
		"app",
		"application",
		"1",
		"",
	}, "\n"))
	var out bytes.Buffer
	selection, err := guidedCreateStackProfile(bufio.NewReader(input), &out, catalog, registry)
	if err != nil {
		t.Fatal(err)
	}
	if !selection.Created || selection.RawProfile == nil {
		t.Fatalf("wizard did not create reusable profile: %#v", selection)
	}
	if selection.SaveScope != development.ProfileScopeUser {
		t.Fatalf("scope = %q, want user", selection.SaveScope)
	}
	if selection.RawProfile.Metadata.Name != "team-stack" {
		t.Fatalf("name = %q", selection.RawProfile.Metadata.Name)
	}
	if len(selection.Effective.Components) != 1 || selection.Effective.Components[0].Adapter != "development/go" {
		t.Fatalf("unexpected effective profile: %#v", selection.Effective)
	}
}

func TestAppNewWizardControllerCreateOnlyAdapter(t *testing.T) {
	registry, err := referenceDevelopmentRegistry()
	if err != nil {
		t.Fatal(err)
	}
	manifest := application.New("controller-only", "dev", true, false, false)
	manifest.ApplicationID = application.MustNewApplicationID()
	reg := capability.NewRegistry()
	input := appWizardInput{
		Manifest:  manifest,
		Snapshot:  application.ResolutionSnapshot{Target: "dev", Registry: reg},
		Selection: appCapabilitySelectionInput{Current: []capability.Requirement{{Kind: capability.SQL, Name: "default"}}},
		TTY:       true,
		Refresh: func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
			return &GuidedReadContext{Footprint: application.Footprint{Target: d.Target}},
				&application.ProviderImpactPlan{Target: d.Target}, nil
		},
	}
	controller, err := newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "controller-only")
	request := development.NewApplicationRequest{Name: "controller-only", Environment: "dev", Adapter: "development/go", Capabilities: []capability.Kind{capability.SQL}}
	if _, err := appNewCreateFromWizardDecision(context.Background(), controller, root, request, registry); err == nil {
		t.Fatal("unconfirmed selection created a project")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("early project write")
	}
	controller, err = controller.Step(context.Background(), appWizardNext)
	if err != nil {
		t.Fatal(err)
	}
	controller, err = controller.Step(context.Background(), appWizardYes)
	if err != nil {
		t.Fatal(err)
	}
	result, err := appNewCreateFromWizardDecision(context.Background(), controller, root, request, registry)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.ApplicationID != manifest.ApplicationID {
		t.Fatalf("identity changed from confirmed wizard: got %q want %q", result.Manifest.ApplicationID, manifest.ApplicationID)
	}
	for _, name := range []string{"baseharbor.yaml", "compose.yaml"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := appNewCreateFromWizardDecision(context.Background(), controller, root, request, registry); err == nil {
		t.Fatal("existing project was silently overwritten")
	}
}

func TestAppNewWizardControllerRejectsStaleRequestAndCancelledDecision(t *testing.T) {
	registry, err := referenceDevelopmentRegistry()
	if err != nil {
		t.Fatal(err)
	}
	manifest := application.New("intent-only", "dev", true, false, false)
	manifest.ApplicationID = application.MustNewApplicationID()
	input := appWizardInput{
		Manifest: manifest, Snapshot: application.ResolutionSnapshot{Target: "dev", Registry: capability.NewRegistry()},
		Selection: appCapabilitySelectionInput{Current: []capability.Requirement{{Kind: capability.SQL, Name: "default"}}},
		TTY:       true,
		Refresh: func(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
			return &GuidedReadContext{Footprint: application.Footprint{Target: "dev"}},
				&application.ProviderImpactPlan{Target: "dev"}, nil
		},
	}
	controller, err := newAppWizardController(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, _ := controller.Step(context.Background(), appWizardCancel)
	root := filepath.Join(t.TempDir(), "cancelled")
	request := development.NewApplicationRequest{Name: "intent-only", Environment: "dev", Adapter: "development/go", Capabilities: []capability.Kind{capability.SQL}}
	if _, err := appNewCreateFromWizardDecision(context.Background(), cancelled, root, request, registry); err == nil {
		t.Fatal("cancelled decision allowed a write")
	}
	controller, err = controller.Step(context.Background(), appWizardNext)
	if err != nil {
		t.Fatal(err)
	}
	controller, err = controller.Step(context.Background(), appWizardYes)
	if err != nil {
		t.Fatal(err)
	}
	request.ApplicationID = application.MustNewApplicationID()
	if _, err := appNewCreateFromWizardDecision(context.Background(), controller, root, request, registry); err == nil {
		t.Fatal("conflicting application ID was accepted")
	}
	request.ApplicationID = ""
	request.Environment = "test"
	if _, err := appNewCreateFromWizardDecision(context.Background(), controller, root, request, registry); err == nil {
		t.Fatal("conflicting environment was accepted")
	}
	request.Environment = "dev"
	request.Capabilities = []capability.Kind{capability.KeyValue}
	if _, err := appNewCreateFromWizardDecision(context.Background(), controller, root, request, registry); err == nil {
		t.Fatal("stale or mismatched capabilities were accepted")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("rejected request wrote project")
	}
}

func TestAppNewWizardFormatting(t *testing.T) {
	for _, name := range []string{"app_new.go", "app_new_wizard.go", "app_new_wizard_test.go"} {
		diff, err := exec.Command("gofmt", "-d", name).CombinedOutput()
		if err != nil || len(diff) > 0 {
			t.Errorf("gofmt mismatch in %s: %s (%v)", name, diff, err)
		}
	}
}
