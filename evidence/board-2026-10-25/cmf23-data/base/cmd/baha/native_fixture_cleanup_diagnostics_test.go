package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Failed native cleanup must remain a failure. Report only public plan IDs and
// field categories, never private material values or their individual hashes.
func logNativeFixtureCleanupMismatch(t *testing.T, planned runtimeDestroyImpact, err error) {
	t.Helper()
	var failure *machine.Error
	if !errors.As(err, &failure) {
		return
	}
	switch current := failure.Details.(type) {
	case runtimeDestroyImpact:
		categories := []string{}
		for _, item := range []struct {
			name            string
			before, current any
		}{
			{"resources", planned.Resources, current.Resources},
			{"preserved", planned.Preserved, current.Preserved},
			{"core", planned.Core, current.Core},
			{"effects", planned.Effects, current.Effects},
			{"local-state", planned.LocalState, current.LocalState},
			{"applications", planned.Applications, current.Applications},
			{"discovery", planned.Discovery, current.Discovery},
		} {
			if !reflect.DeepEqual(item.before, item.current) {
				categories = append(categories, item.name)
			}
		}
		t.Logf("[native-cleanup-scope] aggregate planned=%s current=%s changed-fields=%v", planned.Scope.PlanID, current.Scope.PlanID, categories)
		for _, child := range current.Applications {
			logNativeFixtureChildScope(t, planned, child)
		}
	case applicationDestructionPlan:
		t.Log("[native-cleanup-scope] delegated application scope refused")
		logNativeFixtureChildScope(t, planned, current)
	}
}

func logNativeFixtureChildScope(t *testing.T, planned runtimeDestroyImpact, current applicationDestructionPlan) {
	t.Helper()
	for _, child := range planned.Applications {
		if child.Scope.Owner.Subject != current.Scope.Owner.Subject {
			continue
		}
		t.Logf("[native-cleanup-scope] child planned=%s current=%s materials-changed=%t effects-changed=%t provider-plan-changed=%t", child.Scope.PlanID, current.Scope.PlanID, !reflect.DeepEqual(child.Materials, current.Materials), !reflect.DeepEqual(child.Effects, current.Effects), !reflect.DeepEqual(child.Providers.Impact, current.Providers.Impact))
		if child.Providers.Impact != nil && current.Providers.Impact != nil {
			t.Logf("[native-cleanup-scope] provider planned=%s current=%s", child.Providers.Impact.PlanID, current.Providers.Impact.PlanID)
		}
		return
	}
	t.Log("[native-cleanup-scope] current child has no matching planned deployment")
}

func TestGuidedBackupCleanupDiagnosticRedactsOwnerIdentity(t *testing.T) {
	app := applicationDestructionPlan{}
	app.Scope.Owner.Subject = "deployment-identity-sentinel"
	app.Scope.Owner.Owner = "application-owner-sentinel"
	app.Scope.Owner.Reference = "/private-state-reference-sentinel"
	app.Scope.Owner.Revision = "private-revision-sentinel"
	app.Scope.PlanID = "private-plan-sentinel"
	runtime := runtimeDestroyImpact{Scope: app.Scope}
	runtime.Scope.Owner.Subject = "target-identity-sentinel"
	runtime.Scope.Owner.Owner = "runtime-owner-sentinel"
	runtime.Core = []runtimeDestroyCoreImpact{{
		Target: "target-name-sentinel", Project: "project-name-sentinel",
	}}
	for _, scenario := range []struct {
		name    string
		details any
		owner   string
	}{
		{"application", app, "application-deployment"},
		{"installation", runtime, "installation"},
		{"unknown", map[string]string{"owner": "unknown-owner-sentinel"}, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			failure := &machine.Error{
				Code: machine.ErrorOwnershipAmbiguous, Message: "private-error-sentinel",
				Details: scenario.details,
			}
			diagnostic := nativeFixtureCleanupDiagnostic(fmt.Errorf("wrapped refusal: %w", failure))
			if diagnostic == nil || diagnostic.Caller != "native-fixture-cleanup" ||
				diagnostic.Owner != scenario.owner || diagnostic.Classification != "ownership-ambiguous" {
				t.Fatalf("unexpected bounded cleanup diagnostic: %+v", diagnostic)
			}
			data, err := json.Marshal(diagnostic)
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{
				"deployment-identity-sentinel", "application-owner-sentinel",
				"/private-state-reference-sentinel", "private-revision-sentinel",
				"private-plan-sentinel", "target-identity-sentinel",
				"runtime-owner-sentinel", "target-name-sentinel",
				"project-name-sentinel", "unknown-owner-sentinel", "private-error-sentinel",
			} {
				if strings.Contains(string(data), marker) {
					t.Fatal("cleanup diagnostic exposed a private marker")
				}
			}
		})
	}
}

func TestGuidedBackupCleanupDiagnosticClassificationsStayBounded(t *testing.T) {
	if nativeFixtureCleanupDiagnostic(nil) != nil {
		t.Fatal("successful cleanup produced a diagnostic")
	}
	var absent *machine.Error
	for _, failure := range []error{errors.New("private-error-sentinel"), absent} {
		got := nativeFixtureCleanupDiagnostic(failure)
		if got == nil || got.Owner != "" || got.Classification != "destroy-refused" {
			t.Fatalf("unexpected unclassified diagnostic: %+v", got)
		}
	}
	for _, scenario := range []struct {
		code machine.ErrorCode
		want string
	}{
		{machine.ErrorPolicyDenied, "policy-denied"},
		{machine.ErrorAuthenticationFailed, "authentication-required"},
		{machine.ErrorValidationFailed, "validation-failed"},
		{machine.ErrorCode("private-code-sentinel"), "destroy-refused"},
	} {
		got := nativeFixtureCleanupDiagnostic(&machine.Error{
			Code: scenario.code, Message: "private-error-sentinel",
		})
		if got == nil || got.Owner != "" || got.Classification != scenario.want {
			t.Fatalf("unexpected failure classification: %+v", got)
		}
	}
}
