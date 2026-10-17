package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/creack/pty"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
)

// newWizardPromptReader supplies one input per real prompt. Provider discovery
// may change the number of automatic choices, but the separate execution mode
// and write consent must still be explicitly reached and accepted.
type newWizardPromptReader struct {
	output *bytes.Buffer
	prefix []string
	mode   string
}

func (r *newWizardPromptReader) Read(dst []byte) (int, error) {
	answer := ""
	if len(r.prefix) > 0 {
		answer = r.prefix[0]
		r.prefix = r.prefix[1:]
	} else {
		prompt := r.output.String()
		if i := strings.LastIndexByte(prompt, '\n'); i >= 0 {
			prompt = prompt[i+1:]
		}
		switch {
		case strings.Contains(prompt, "After creation"):
			answer = r.mode
		case strings.Contains(prompt, "Create project files"):
			answer = "y"
		}
	}
	return copy(dst, answer+"\n"), nil
}

func TestAppNewWizardCancelLeavesNoProject(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	configureTestTarget(t)
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())

	oldInput := appNewInput
	defer func() { appNewInput = oldInput }()
	appNewInput = strings.NewReader(strings.Join([]string{
		"cancel-app",
		parent,
		"1",
		"create-only",
		"enter",
		"n",
		"cancel",
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
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	configureTestTarget(t)
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())

	oldInput := appNewInput
	defer func() { appNewInput = oldInput }()
	var out bytes.Buffer
	appNewInput = &newWizardPromptReader{output: &out, prefix: []string{"guided-app", parent, "1"}, mode: "create-only"}
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
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	configureTestTarget(t)
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())
	old := appNewInput
	defer func() { appNewInput = old }()
	var output bytes.Buffer
	appNewInput = &newWizardPromptReader{output: &output, prefix: []string{"identity-preserved", parent, "1"}, mode: "create-only"}
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
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	configureTestTarget(t)
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())
	old := appNewInput
	defer func() { appNewInput = old }()
	appNewInput = strings.NewReader(strings.Join([]string{
		"cancel-at-confirm", parent, "1", "create-only", "enter",
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
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	configureTestTarget(t)
	registry, err := referenceDevelopmentRegistry()
	if err != nil {
		t.Fatal(err)
	}
	manifest := application.New("controller-only", "dev", false, false, false)
	manifest.ApplicationID = application.MustNewApplicationID()
	manifest = application.WithSQLInstances(manifest, "primary")
	reg := capability.NewRegistry()
	input := appWizardInput{
		Manifest:  manifest,
		Snapshot:  application.ResolutionSnapshot{Target: "docker-dev", Registry: reg},
		Selection: appCapabilitySelectionInput{Current: []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}},
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
	manifestBytes, err := os.ReadFile(filepath.Join(root, "baseharbor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifestBytes), "primary") {
		t.Fatalf("named requirement was lost: %s", manifestBytes)
	}
	if _, err := appNewCreateFromWizardDecision(context.Background(), controller, root, request, registry); err == nil {
		t.Fatal("existing project was silently overwritten")
	}
}

func TestAppNewWizardControllerRejectsStaleRequestAndCancelledDecision(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	configureTestTarget(t)
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
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("rejected request wrote project")
	}
}

func TestAppNewWizardRunReportsCreatedProjectWhenLifecyclePartiallyFails(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	configureTestTarget(t)
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())
	oldInput := appNewInput
	oldExecutor := appNewLifecycleExecutor
	defer func() {
		appNewInput = oldInput
		appNewLifecycleExecutor = oldExecutor
	}()
	var out bytes.Buffer
	appNewInput = &newWizardPromptReader{output: &out, prefix: []string{"partial-run", parent, "1"}, mode: "run"}
	appNewLifecycleExecutor = func(context.Context, application.Store, []string, io.Writer, io.Writer) error {
		return errors.New("fixture lifecycle failure")
	}
	err := runAppNewWizard(context.Background(), &out, &out)
	if err == nil || !strings.Contains(err.Error(), "fixture lifecycle failure") {
		t.Fatalf("partial lifecycle failure missing: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(parent, "partial-run", "baseharbor.yaml")); statErr != nil {
		t.Fatalf("confirmed project should remain after lifecycle partial failure: %v", statErr)
	}
	if !strings.Contains(out.String(), "Project files were created; the authorized lifecycle did not complete.") {
		t.Fatalf("partial result was not presented:\n%s", out.String())
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

// Script the actual New entry by prompt, including repeated back navigation.
type newWizardScenario struct {
	output        *bytes.Buffer
	prefix        []string
	actions       []string
	confirmations []string
	onConfirm     func(int)
	count         int
}

func (r *newWizardScenario) Read(dst []byte) (int, error) {
	answer := ""
	if len(r.prefix) > 0 {
		answer = r.prefix[0]
		r.prefix = r.prefix[1:]
	} else {
		prompt := r.output.String()
		if i := strings.LastIndexByte(prompt, '\n'); i >= 0 {
			prompt = prompt[i+1:]
		}
		switch {
		case strings.Contains(prompt, "After creation"):
			answer = "create-only"
		case strings.Contains(prompt, "Selection action"):
			if len(r.actions) == 0 {
				return 0, io.EOF
			}
			answer = r.actions[0]
			r.actions = r.actions[1:]
		case strings.Contains(prompt, "Create project files"):
			r.count++
			if r.onConfirm != nil {
				r.onConfirm(r.count)
			}
			if len(r.confirmations) == 0 {
				return 0, io.EOF
			}
			answer = r.confirmations[0]
			r.confirmations = r.confirmations[1:]
		default:
			return 0, io.EOF
		}
	}
	return copy(dst, answer+"\n"), nil
}

func TestAppNewWizardRepeatedNoPreservesEditableChecklist(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	configureTestTarget(t)
	parent := t.TempDir()
	var out bytes.Buffer
	old := appNewInput
	defer func() { appNewInput = old }()
	scenario := &newWizardScenario{output: &out, prefix: []string{"back-loop", parent, "1"}, actions: []string{"space", "enter", "space", "enter", "enter"}, confirmations: []string{"n", "n", "y"}}
	scenario.onConfirm = func(n int) {
		if n < 3 {
			if _, err := os.Stat(filepath.Join(parent, "back-loop")); !os.IsNotExist(err) {
				t.Fatal("preview wrote project before confirmation")
			}
		}
	}
	appNewInput = scenario
	if err := runAppNewWizard(context.Background(), &out, &out); err != nil {
		t.Fatal(err)
	}
	if scenario.count != 3 {
		t.Fatal("repeated No did not return to selection")
	}
	if strings.Count(out.String(), "Return to the preserved checklist") != 2 {
		t.Fatal(out.String())
	}
	selection, err := application.ResolveRepositoryEnvironment(filepath.Join(parent, "back-loop"), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Manifest.Services.SQL {
		t.Fatal("second selection did not remove SQL")
	}
}

func TestAppNewWizardEmptyNeedsCreateOnlyDoesNotRun(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	configureTestTarget(t)
	parent := t.TempDir()
	var out bytes.Buffer
	old := appNewInput
	executor := appNewLifecycleExecutor
	defer func() { appNewInput = old; appNewLifecycleExecutor = executor }()
	appNewLifecycleExecutor = func(context.Context, application.Store, []string, io.Writer, io.Writer) error {
		t.Fatal("create-only executed lifecycle")
		return nil
	}
	appNewInput = &newWizardScenario{output: &out, prefix: []string{"empty-needs", parent, "1"}, actions: []string{"down", "down", "down", "down", "space", "enter"}, confirmations: []string{"y"}}
	if err := runAppNewWizard(context.Background(), &out, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	selection, err := application.ResolveRepositoryEnvironment(filepath.Join(parent, "empty-needs"), "dev")
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	contract, err := application.PortableContractFromManifest(selection.Manifest)
	if err != nil || len(contract.Capabilities) != 0 || contract.Secrets.Managed {
		t.Fatal("empty selection manufactured needs", err)
	}
}

func TestAppNewWizardPersistsExactExplicitPreferenceForLaterResolution(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	selected := capability.ProviderInstance{ID: "postgresql/shared/custom", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, SharingBoundary: "team:custom", Ownership: capability.OwnershipBaseHarbor}
	probe := application.New("probe", "dev", true, false, false)
	probe.ApplicationID = application.MustNewApplicationID()
	probe, err = application.WithBackendPlacementProjection(probe, map[capability.ProviderKind]capability.ProviderPlacement{capability.ProviderPostgreSQL: {Scope: selected.Scope, SharingBoundary: selected.SharingBoundary, Ownership: selected.Ownership}})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := application.PreviewReferenceProviderRegistryAt(root, probe)
	if err != nil {
		t.Fatal(err)
	}
	for _, instance := range canonical.Instances {
		if instance.Provider.Kind == capability.ProviderPostgreSQL {
			selected = instance
		}
	}
	if err := registry.Register(selected); err != nil {
		t.Fatal(err)
	}
	if err := (capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}).Save(registry); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	var out bytes.Buffer
	old := appNewInput
	defer func() { appNewInput = old }()
	appNewInput = &newWizardScenario{output: &out, prefix: []string{"exact-provider", parent, "1"}, actions: []string{"space", "provider database.sql/default " + selected.ID, "enter"}, confirmations: []string{"y"}}
	if err := runAppNewWizard(context.Background(), &out, &out); err != nil {
		t.Fatal(err)
	}
	records, err := deployment.ListDeployments(target.Name)
	if err != nil || len(records) != 1 {
		t.Fatal("missing configured owner", records, err)
	}
	var m application.Manifest
	if err := json.Unmarshal(records[0].Applied.Intent, &m); err != nil {
		t.Fatal(err)
	}
	prefs, err := application.DesiredProviderPreferencesAt(root, m)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pref := range prefs {
		if pref.Requirement.Kind == capability.SQL && pref.InstanceID == selected.ID && pref.ProviderKind == selected.Provider.Kind {
			found = true
		}
	}
	if !found {
		t.Fatalf("exact preference not retained: %+v", prefs)
	}
	planned, err := application.PreviewReferenceProviderRegistryAt(root, m)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, binding := range planned.Bindings {
		if binding.Resource.Kind == capability.SQL && binding.ProviderInstanceID == selected.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("later registry preview ignored exact preference")
	}
	// Resolve through the normal later app entry, not merely a registry helper.
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(parent, "exact-provider")); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "plan")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Manifest.ApplicationID != m.ApplicationID {
		t.Fatal("later entry changed the confirmed system identity")
	}
	placement, err := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderPostgreSQL)
	if err != nil || placement.SharingBoundary != selected.SharingBoundary {
		t.Fatal("later native entry ignored desired placement", placement, err)
	}
	if records[0].Observed.Ready || !records[0].Observed.VerifiedAt.IsZero() {
		t.Fatal("create-only claimed runtime verified")
	}
	source, err := os.ReadFile(filepath.Join(parent, "exact-provider", "baseharbor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), selected.ID) {
		t.Fatal("Target provider leaked into portable source")
	}
}

func TestAppNewWizardStaleExplicitProviderWritesNoProject(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	chosen := capability.ProviderInstance{ID: "postgresql/shared/selected", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, SharingBoundary: "team:selected", Ownership: capability.OwnershipBaseHarbor}
	probe := application.New("probe", "dev", true, false, false)
	probe.ApplicationID = application.MustNewApplicationID()
	probe, err = application.WithBackendPlacementProjection(probe, map[capability.ProviderKind]capability.ProviderPlacement{capability.ProviderPostgreSQL: {Scope: chosen.Scope, SharingBoundary: chosen.SharingBoundary, Ownership: chosen.Ownership}})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := application.PreviewReferenceProviderRegistryAt(root, probe)
	if err != nil {
		t.Fatal(err)
	}
	for _, instance := range canonical.Instances {
		if instance.Provider.Kind == capability.ProviderPostgreSQL {
			chosen = instance
		}
	}
	if err := registry.Register(chosen); err != nil {
		t.Fatal(err)
	}
	store := capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	var out bytes.Buffer
	old := appNewInput
	defer func() { appNewInput = old }()
	scenario := &newWizardScenario{output: &out, prefix: []string{"stale-choice", parent, "1"}, actions: []string{"space", "provider database.sql/default " + chosen.ID, "enter"}, confirmations: []string{"y"}}
	scenario.onConfirm = func(int) {
		if err := store.Save(capability.NewRegistry()); err != nil {
			t.Fatal(err)
		}
	}
	appNewInput = scenario
	if err := runAppNewWizard(context.Background(), &out, &out); err == nil {
		t.Fatal("stale explicit selection accepted")
	}
	if _, err := os.Stat(filepath.Join(parent, "stale-choice")); !os.IsNotExist(err) {
		t.Fatal("stale selection wrote project")
	}
	records, err := deployment.ListDeployments(target.Name)
	if err != nil || len(records) != 0 {
		t.Fatal("stale selection wrote deployment/preference state", err)
	}
}

func TestAppNewWizardActualTerminalSelectionUsesArrowSpaceAndEnter(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	configureTestTarget(t)
	t.Setenv("TERM", "xterm-256color")
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := pty.Setsize(master, &pty.Winsize{Rows: 40, Cols: 120}); err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	old := appNewInput
	appNewInput = slave
	defer func() { appNewInput = old }()
	m := application.New("terminal-needs", "dev", true, false, false)
	m.ApplicationID = application.MustNewApplicationID()
	m = application.WithWorkloadComponents(m, "app")
	_, snapshot, err := appNewWizardAuthorityContext(context.Background(), "", m)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := newAppWizardController(context.Background(), appWizardInput{Manifest: m, Snapshot: snapshot, Selection: appCapabilitySelectionInput{Current: []capability.Requirement{{Kind: capability.SQL, Name: "default"}}}, TTY: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type result struct {
		controller appWizardController
		err        error
	}
	finished := make(chan result, 1)
	go func() {
		c, e := appNewEditSelection(ctx, bufio.NewReader(slave), slave, controller)
		finished <- result{c, e}
	}()
	ready := make(chan struct{}, 1)
	go func() {
		var rendered strings.Builder
		buf := make([]byte, 4096)
		for {
			n, e := master.Read(buf)
			rendered.Write(buf[:n])
			if strings.Contains(rendered.String(), "What does your application need?") {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
			if e != nil {
				return
			}
		}
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("terminal selection did not render")
	}
	// Move to Cache and select it without a newline, then confirm with Enter.
	if _, err := master.Write([]byte("\x1b[B \r")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-finished:
		if got.err != nil {
			t.Fatal(got.err)
		}
		reqs := got.controller.selection.Requirements()
		if len(reqs) != 2 {
			t.Fatalf("real terminal keys did not preserve SQL and select Cache: %+v", reqs)
		}
	case <-ctx.Done():
		t.Fatal("terminal selection did not finish")
	}
}
