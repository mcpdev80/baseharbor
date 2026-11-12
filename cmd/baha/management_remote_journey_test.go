package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/sourcetransfer"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// Real pinned HTTPS, Machine execution journals and source transfers; the
// executor models Core lifecycle receipts without using a workstation runtime.
type remoteJourneyCore struct {
	mu                  sync.Mutex
	transfers           *sourcetransfer.Store
	id                  string
	intent              application.Manifest
	plan                application.ProviderImpactPlan
	operations          []string
	applyCalls, applied int
	applyFailure        error
	candidateChanged    bool
	previewOverride     func(*applicationLifecyclePlan)
	developmentDomain   string
	noProviderChanges   bool
	readResults         map[string]json.RawMessage
}

func (f *remoteJourneyCore) SupportedOperationIDs() []string {
	return []string{"app.list", "app.create", "app.configure", "apply", "dev.domain", "status", "doctor", "control-plane.status", "control-plane.doctor"}
}
func (f *remoteJourneyCore) BindSourceTransfers(store *sourcetransfer.Store) { f.transfers = store }
func (f *remoteJourneyCore) Execute(_ context.Context, op machine.Operation, scope machine.OperationContext, raw json.RawMessage, report machinehttp.ProgressReporter) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.operations = append(f.operations, op.ID)
	if scope.Target != "local" || scope.Environment != "dev" {
		return nil, errors.New("wrong Core selector")
	}
	encode := func(value any) (json.RawMessage, error) { return json.Marshal(value) }
	if result, ok := f.readResults[op.ID]; ok {
		return result, nil
	}
	switch op.ID {
	case "dev.domain":
		var input machineHTTPDevelopmentDomainInput
		if json.Unmarshal(raw, &input) != nil || input.Target != "local" {
			return nil, errors.New("domain request escaped selected Core")
		}
		domain := f.developmentDomain
		if domain == "" {
			domain = "core.example.test"
		}
		if input.Domain != "" {
			domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(input.Domain), "."))
			f.developmentDomain = domain
		}
		return encode(machineHTTPDevelopmentDomainResult{ContractVersion: machine.ContractVersion, Target: "local", Environment: "dev", Domain: domain})
	case "app.list":
		list := machineApplicationListResult{ContractVersion: machine.ContractVersion, Deployments: []machineDeploymentSummary{}}
		if f.id != "" {
			list.Deployments = append(list.Deployments, machineDeploymentSummary{ApplicationID: f.id, Target: "local", Application: "demo", Environment: "dev"})
		}
		return encode(list)
	case "app.create":
		var input machineApplicationCreateInput
		if json.Unmarshal(raw, &input) != nil || input.Intent.ApplicationID != "" || f.id != "" {
			return nil, errors.New("invalid or duplicate remote registration")
		}
		f.id, f.intent = application.MustNewApplicationID(), input.Intent
		return encode(applicationAdoptionResult{Application: "demo", ApplicationID: f.id, Environment: "dev", Manifest: "/core/owned/manifest"})
	case "app.configure", "apply":
		var input machineApplyInput
		if json.Unmarshal(raw, &input) != nil || input.Name != "demo" || f.id == "" {
			return nil, errors.New("application was not registered")
		}
		current, _, err := f.transfers.Current("local", "demo", "dev")
		if err != nil || current.Digest != input.SourceSnapshotDigest {
			return nil, errors.New("source transfer did not bind exact repository bytes")
		}
		if op.ID == "apply" {
			f.applyCalls++
			if f.applyFailure != nil {
				return nil, f.applyFailure
			}
			var consent application.ProviderChangeConsent
			if input.ProviderChangeConsent != nil {
				consent = *input.ProviderChangeConsent
			}
			consent.PlanID = firstNonEmpty(consent.PlanID, f.plan.PlanID)
			if err := application.ValidateProviderChangeConsent(f.plan, consent); err != nil && !f.noProviderChanges {
				failure := providerImpactConsentError(f.plan, err)
				failure.(*machine.Error).Next += " Candidate: --confirm-candidate " + strings.Repeat("a", 64)
				return nil, failure
			}
			digest := strings.Repeat("a", 64)
			if f.candidateChanged {
				digest = strings.Repeat("b", 64)
			}
			if input.ConfirmCandidate != digest {
				preview := applicationLifecyclePlan{Plan: application.Plan{ContractVersion: "v1", Application: "demo", Environment: "dev"}, Footprint: application.Footprint{Version: application.FootprintVersion, Target: "local"}, ProviderPlan: application.ProviderPlan{Version: application.ProviderPlanVersion, Target: "local"}}
				contract, _ := application.PortableContractFromManifest(f.intent)
				for _, requirement := range contract.Capabilities {
					state := application.ProviderRunning
					if requirement.Kind == capability.KeyValue {
						state = application.ProviderNewRequired
					}
					preview.Footprint.Requirements = append(preview.Footprint.Requirements, application.RequirementResolution{Requirement: requirement, State: state})
				}
				if f.previewOverride != nil {
					f.previewOverride(&preview)
				}
				return nil, &machine.Error{Code: machine.ErrorApprovalRequired, CauseCode: "candidate_confirmation_required", Message: "Candidate approval required", Details: preview, Next: "Re-run baha app apply demo --confirm-candidate " + digest}
			}
			f.applied++
		}
		report(machine.OperationProgress{Stage: "verify", Message: "Verifying remote application state."})
		return encode(map[string]any{"contract_version": machine.ContractVersion, "operation": op.ID, "verified": true, "target": "local", "application": "demo", "environment": "dev", "status": map[string]any{"target": "local", "application": "demo", "environment": "dev", "ready": true}})
	}
	return nil, errors.New("unsupported operation")
}

func remoteJourneyFixture(t *testing.T) (context.Context, deployment.ResolvedTarget, *remoteJourneyCore) {
	t.Helper()
	ctx, _, target := remoteApplicationCoreFixtureEmpty(t)
	if err := configureCoreRemoteTargetForTest(target.Name); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_TARGET", target.Name)
	var err error
	target, err = effectiveTarget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := &remoteJourneyCore{plan: application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "local", PlanID: "current-plan", ConfirmationsRequired: 2, Impacts: []application.ProviderImpact{{InstanceID: "postgres", Operation: "add"}}}}
	handler, err := machinehttp.New(f, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-session" {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.URL.Path == "/api/v1/management/installation" {
			json.NewEncoder(w).Encode(map[string]string{"version": managementInstallationVersion, "installation_id": "test-installation", "identity_issuer": "https://core.example", "operator_issuer": "https://issuer.example", "tenant_id": target.TenantID})
			return
		}
		principal := &identity.Principal{Issuer: "https://issuer.example", Subject: "test-operator", IssuerVerified: true, Groups: []string{"editor"}}
		auth := identity.WithPrincipal(r.Context(), principal)
		auth = operatorauth.WithVerifiedPrincipal(auth, principal)
		auth = operatorauth.WithMembershipProvider(auth, &firstRunMembershipProvider{memberships: []tenancy.Membership{{TenantID: target.TenantID, Role: "editor"}}})
		handler.ServeHTTP(w, r.WithContext(auth))
	}))
	t.Cleanup(server.Close)
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	digest := sha256.Sum256(cert)
	profile := map[string]any{"version": remoteConnectionProfileVersion, "name": "journey", "core": map[string]string{"installation_id": "test-installation", "url": server.URL}, "trust": map[string]string{"ca_pem": string(cert), "sha256": hex.EncodeToString(digest[:])}, "targets": []map[string]string{{"name": target.Name, "core_target": "local", "tenant_id": target.TenantID, "runtime_provider": target.RuntimeProvider}}}
	data, _ := json.Marshal(profile)
	path := filepath.Join(t.TempDir(), "connection.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_CONNECTION_PROFILE", path)
	if err := operatorauth.SaveSession(operatorauth.Session{Target: target.Name, Environment: "dev", Issuer: "https://issuer.example", ClientID: "baha-cli", Subject: "test-operator", IDToken: "test-session", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "api")
	if err := os.WriteFile("baseharbor.yaml", []byte(m.AuthoredYAML()), 0600); err != nil {
		t.Fatal(err)
	}
	return ctx, target, f
}

func TestRemoteRepositoryInitThenBareUpUsesCoreIdentityAndPrompts(t *testing.T) {
	ctx, _, f := remoteJourneyFixture(t)
	old := runtimeInput
	runtimeInput = strings.NewReader("y\n")
	t.Cleanup(func() { runtimeInput = old })
	var output, errorsOut bytes.Buffer
	if err := appInitWithInputResolverCommand(application.DefaultStore()).Run(ctx, nil, &output, &errorsOut); err != nil {
		t.Fatal(err)
	}
	if err := runtimeUpCommandWithInputResolver(ctx, nil, &output, &errorsOut); err != nil {
		t.Fatalf("bare remote up failed: %v\n%s\n%s", err, output.String(), errorsOut.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.applied != 1 || f.applyCalls != 2 {
		t.Fatalf("wrong lifecycle attempts: applied=%d calls=%d", f.applied, f.applyCalls)
	}
	if strings.Count(strings.Join(f.operations, ","), "app.create") != 1 {
		t.Fatalf("registration repeated: %v", f.operations)
	}
	if strings.Count(errorsOut.String(), "[y/N]") != 1 {
		t.Fatalf("expected one confirmation, got: %s", errorsOut.String())
	}
	for _, text := range []string{"Check application registration", "Transfer source", "Installation on", "Deploy application from the current source", "Verifying remote application state."} {
		if !strings.Contains(output.String()+errorsOut.String(), text) {
			t.Errorf("missing progress/consent %q", text)
		}
	}
}

func TestRemoteRepositoryInitJSONCreatesAndReturnsOnlyCoreIdentity(t *testing.T) {
	ctx, _, f := remoteJourneyFixture(t)
	if err := os.Remove("baseharbor.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	var output, errorsOut bytes.Buffer
	err := appInitWithInputResolverCommand(application.DefaultStore()).Run(ctx, []string{"demo", "--workload-component", "api", "--json"}, &output, &errorsOut)
	if err != nil {
		t.Fatalf("remote contract init failed: %v\n%s", err, errorsOut.String())
	}
	var result applicationAdoptionResult
	if json.Unmarshal(output.Bytes(), &result) != nil || result.ApplicationID == "" || result.ApplicationID != f.id {
		t.Fatalf("JSON init did not return Core-assigned identity: %s", output.String())
	}
	if errorsOut.Len() != 0 {
		t.Fatal("JSON init emitted human progress")
	}
	data, err := os.ReadFile("baseharbor.yaml")
	if err != nil || bytes.Contains(data, []byte(f.id)) {
		t.Fatal("system identity leaked into authored source")
	}
}

func TestRemoteRepositoryApplyNeverRetriesUnrelatedOrStaleFailure(t *testing.T) {
	for _, cause := range []string{"stale-impact-consent", "unknown-outcome", "unrelated-approval"} {
		t.Run(cause, func(t *testing.T) {
			ctx, _, f := remoteJourneyFixture(t)
			code := machine.ErrorConflict
			if cause == "unrelated-approval" {
				code = machine.ErrorApprovalRequired
			}
			f.applyFailure = &machine.Error{Code: code, CauseCode: cause, Message: cause}
			old := runtimeInput
			runtimeInput = strings.NewReader("y\ny\ny\n")
			t.Cleanup(func() { runtimeInput = old })
			var output bytes.Buffer
			err := runtimeUpCommandWithInputResolver(ctx, nil, &output, &output)
			if err == nil {
				t.Fatal("failed lifecycle reported success")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.applyCalls != 1 || f.applied != 0 {
				t.Fatalf("unsafe mutation retry: %d/%d", f.applyCalls, f.applied)
			}
		})
	}
}

func TestRemoteRepositoryDestructionRequiresExplicitDeletionConsent(t *testing.T) {
	ctx, _, f := remoteJourneyFixture(t)
	f.plan.Destructive = true
	old := runtimeInput
	runtimeInput = strings.NewReader("n\n")
	t.Cleanup(func() { runtimeInput = old })
	var output bytes.Buffer
	err := runtimeUpCommandWithInputResolver(ctx, nil, &output, &output)
	var denied *machine.Error
	if !errors.As(err, &denied) || denied.CauseCode != "impact-confirmation-required" || !strings.Contains(output.String(), "permanently delete the listed data") {
		t.Fatalf("missing independent deletion consent: %v\n%s", err, output.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.applied != 0 || f.applyCalls != 1 {
		t.Fatal("acknowledgement stages approved data deletion")
	}
}

func TestRemoteRepositoryRejectsForeignOrRepeatedPlan(t *testing.T) {
	for _, scenario := range []string{"foreign", "repeated"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, _, f := remoteJourneyFixture(t)
			if scenario == "foreign" {
				f.plan.Target = "another-core"
			} else {
				f.applyFailure = providerImpactConsentError(f.plan, &application.ProviderChangeError{Code: "impact-confirmation-required"})
			}
			old := runtimeInput
			runtimeInput = strings.NewReader("y\ny\ny\ny\n")
			t.Cleanup(func() { runtimeInput = old })
			var output bytes.Buffer
			if err := runtimeUpCommandWithInputResolver(ctx, nil, &output, &output); err == nil {
				t.Fatal("untrusted/repeated plan accepted")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			max := 1
			if scenario == "repeated" {
				max = 2
			}
			if f.applied != 0 || f.applyCalls != max {
				t.Fatalf("unexpected retries: %d/%d", f.applied, f.applyCalls)
			}
		})
	}
}

func TestRemoteRepositoryGenericYesDoesNotApproveImpact(t *testing.T) {
	ctx, _, f := remoteJourneyFixture(t)
	old := runtimeInput
	runtimeInput = strings.NewReader("n\n")
	t.Cleanup(func() { runtimeInput = old })
	var output bytes.Buffer
	err := runtimeUpCommandWithInputResolver(ctx, []string{"--yes"}, &output, &output)
	if err == nil {
		t.Fatal("generic --yes approved provider changes")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.applied != 0 || f.applyCalls != 1 {
		t.Fatalf("declined plan retried mutation: %d/%d", f.applied, f.applyCalls)
	}
}

func TestRemoteRepositoryNoInputReturnsChallengeAsStructuredError(t *testing.T) {
	ctx, _, f := remoteJourneyFixture(t)
	var output, errorsOut bytes.Buffer
	err := runtimeUpCommandWithInputResolver(machineNoninteractiveContext(ctx), []string{"--json"}, &output, &errorsOut)
	var denied *machine.Error
	if !errors.As(err, &denied) || denied.CauseCode != "impact-confirmation-required" {
		t.Fatalf("missing typed challenge: %v", err)
	}
	if output.Len() != 0 || errorsOut.Len() != 0 {
		t.Fatal("JSON/noninteractive command emitted progress or prompts")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.applyCalls != 1 || f.applied != 0 {
		t.Fatal("noninteractive apply retried or bypassed consent")
	}
}

func TestRemoteRepositoryRefusesForeignIdentity(t *testing.T) {
	ctx, _, f := remoteJourneyFixture(t)
	f.id = application.MustNewApplicationID()
	var output bytes.Buffer
	err := appInitWithInputResolverCommand(application.DefaultStore()).Run(ctx, nil, &output, &output)
	if err == nil || !strings.Contains(err.Error(), "identity differs") {
		t.Fatalf("adopted foreign application by name: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if fmt.Sprint(f.operations) != "[app.list]" {
		t.Fatalf("foreign application mutated: %v", f.operations)
	}
}

func TestRemoteRepositorySourceChangeCannotReuseCombinedConfirmation(t *testing.T) {
	ctx, _, f := remoteJourneyFixture(t)
	f.candidateChanged = true
	old := runtimeInput
	runtimeInput = strings.NewReader("y\n")
	t.Cleanup(func() { runtimeInput = old })
	var output bytes.Buffer
	err := runtimeUpCommandWithInputResolver(ctx, nil, &output, &output)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.CauseCode != "candidate_confirmation_required" {
		t.Fatalf("changed source was not rejected: %v", err)
	}
	if strings.Count(output.String(), "[y/N]") != 1 || f.applyCalls != 2 || f.applied != 0 {
		t.Fatal("source change reused consent or prompted again")
	}
}

func TestRemoteRepositoryResumeShowsServicesBeforeSingleSourceConfirmation(t *testing.T) {
	ctx, _, f := remoteJourneyFixture(t)
	f.noProviderChanges = true
	m := application.WithWorkloadComponents(application.New("demo", "dev", true, true, false), "api")
	if err := os.WriteFile("baseharbor.yaml", []byte(m.AuthoredYAML()), 0600); err != nil {
		t.Fatal(err)
	}
	old := runtimeInput
	runtimeInput = strings.NewReader("y\n")
	t.Cleanup(func() { runtimeInput = old })
	var output bytes.Buffer
	if err := runtimeUpCommandWithInputResolver(ctx, nil, &output, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "Deployment changes on") || !strings.Contains(text, "Reuse (already present): SQL") || !strings.Contains(text, "Install: Cache") || strings.Count(text, "[y/N]") != 1 {
		t.Fatalf("resumed application lacks one readable decision: %s", text)
	}
	if f.applied != 1 || f.applyCalls != 2 {
		t.Fatal("resumed application did not deploy exactly once")
	}
}

func TestRemoteRepositoryDeclineExplainsCancellationWithoutManualHashInstructions(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", ""} {
		t.Run(fmt.Sprintf("answer-%q", answer), func(t *testing.T) {
			ctx, _, f := remoteJourneyFixture(t)
			f.noProviderChanges = true
			old := runtimeInput
			runtimeInput = strings.NewReader(answer)
			t.Cleanup(func() { runtimeInput = old })
			var output bytes.Buffer
			err := runtimeUpCommandWithInputResolver(ctx, nil, &output, &output)
			if !cli.IsPresented(err) {
				t.Fatalf("cancellation left a raw error for main to print: %v", err)
			}
			if !strings.Contains(output.String(), "Deployment cancelled") || !strings.Contains(output.String(), "run baha up") || strings.Contains(output.String(), "--confirm-candidate") {
				t.Fatalf("obsolete cancellation guidance: %s", output.String())
			}
			if f.applyCalls != 1 || f.applied != 0 {
				t.Fatal("declined source confirmation retried rollout")
			}
		})
	}
}

func TestRemoteRepositoryRejectsDeploymentPreviewForDifferentScope(t *testing.T) {
	for _, scenario := range []string{"target", "environment", "application"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, _, f := remoteJourneyFixture(t)
			f.noProviderChanges = true
			f.previewOverride = func(p *applicationLifecyclePlan) {
				switch scenario {
				case "target":
					p.Footprint.Target = "other"
					p.ProviderPlan.Target = "other"
				case "environment":
					p.Environment = "prod"
				case "application":
					p.Application = "other"
				}
			}
			old := runtimeInput
			runtimeInput = strings.NewReader("y\n")
			t.Cleanup(func() { runtimeInput = old })
			var output bytes.Buffer
			if err := runtimeUpCommandWithInputResolver(ctx, nil, &output, &output); err == nil {
				t.Fatal("foreign preview accepted")
			}
			if strings.Contains(output.String(), "[y/N]") || f.applyCalls != 1 || f.applied != 0 {
				t.Fatal("foreign scope obtained confirmation or execution")
			}
		})
	}
}

func TestRemoteRepositoryJSONCompletionPreservesMachineReceipt(t *testing.T) {
	ctx, _, f := remoteJourneyFixture(t)
	f.noProviderChanges = true
	var output, errorsOut bytes.Buffer
	if err := runtimeUpCommandWithInputResolver(ctx, []string{"--json", "--confirm-candidate", strings.Repeat("a", 64)}, &output, &errorsOut); err != nil {
		t.Fatal(err)
	}
	var receipt map[string]json.RawMessage
	if json.Unmarshal(output.Bytes(), &receipt) != nil || string(receipt["verified"]) != "true" || string(receipt["operation"]) != `"apply"` {
		t.Fatalf("JSON success receipt changed: %s", output.String())
	}
	if errorsOut.Len() != 0 || strings.Contains(output.String(), "READY ·") {
		t.Fatal("human completion leaked into JSON")
	}
}
