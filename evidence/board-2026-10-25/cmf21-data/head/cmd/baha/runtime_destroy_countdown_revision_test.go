package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func retainedCountdownChild(t *testing.T, subject string, remaining int64, revision string) applicationDestructionPlan {
	t.Helper()
	leaf := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "isolated", PlanID: "verified-provider-authority-" + revision, ConfirmationsRequired: 2, Attention: "Achtung", Impacts: []application.ProviderImpact{{InstanceID: "retained-identity", Materials: []application.MaterialConsequence{{RemainingValiditySeconds: &remaining}}, Functions: []application.FunctionConsequence{{RemainingValiditySeconds: &remaining}}}}}
	scope, err := application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "application-deployment", Reference: "/owned/" + subject, Subject: subject, Revision: revision, Destructive: true}, []application.ProviderImpactPlan{leaf}, false)
	if err != nil {
		t.Fatal(err)
	}
	return applicationDestructionPlan{Scope: scope}
}

func installationCountdownScope(t *testing.T, plan runtimeDestroyImpact, owners map[string]string) application.ProviderImpactScope {
	t.Helper()
	scope, err := application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "installation.destroy", Reference: "/owned/config", Subject: "installation.destroy", Revision: runtimeDestroyImpactRevision(plan, owners, nil), Destructive: true}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func TestInstallationConsentTracksCanonicalChildAuthorityNotCertificateCountdown(t *testing.T) {
	before := runtimeDestroyImpact{All: true, Applications: []applicationDestructionPlan{retainedCountdownChild(t, "shared-authority", 120, "original-shared"), retainedCountdownChild(t, "application-authority", 120, "original-application")}}
	current := runtimeDestroyImpact{All: true, Applications: []applicationDestructionPlan{retainedCountdownChild(t, "shared-authority", 117, "original-shared"), retainedCountdownChild(t, "application-authority", 116, "original-application")}}
	for i := range before.Applications {
		if before.Applications[i].Scope.PlanID != current.Applications[i].Scope.PlanID {
			t.Fatal("fixture changed canonical child authority")
		}
	}
	owners := map[string]string{"registry": "protected-owner"}
	planned := installationCountdownScope(t, before, owners)
	ack := application.ProviderChangeConsent{PlanID: planned.PlanID, First: true, Second: true, DeleteData: true}
	if err := application.ValidateProviderImpactScopeConsent(installationCountdownScope(t, current, owners), ack); err != nil {
		t.Fatal("unchanged child authority rejected after certificate countdown resampling", err)
	}
	for _, change := range []string{"child-owner-material", "installation-owner", "selected-resource"} {
		t.Run(change, func(t *testing.T) {
			changed := runtimeDestroyImpact{All: true, Applications: append([]applicationDestructionPlan(nil), current.Applications...)}
			changedOwners := map[string]string{"registry": "protected-owner"}
			switch change {
			case "child-owner-material":
				changed.Applications[1] = retainedCountdownChild(t, "application-authority", 116, "changed-material")
			case "installation-owner":
				changedOwners["registry"] = "changed-protected-owner"
			case "selected-resource":
				changed.Resources = []ownedDestroyResource{{Target: "isolated", Project: "owned", Kind: "volume", Name: "additional-owned-data"}}
			}
			if err := application.ValidateProviderImpactScopeConsent(installationCountdownScope(t, changed, changedOwners), ack); err == nil {
				t.Fatal("changed authority accepted earlier installation consent")
			}
		})
	}
}
