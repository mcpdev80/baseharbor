package application

import (
	"testing"
)

func TestBuildPlanIncludesManagedIdentity(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.SQL = false
	m.Services.Identity = true
	m.Identity.Scopes = []string{"openid", "profile"}

	plan, err := BuildPlan(m)
	if err != nil {
		t.Fatal(err)
	}
	var ensured, verified bool
	for _, action := range plan.Actions {
		if action.Resource != "identity:default" {
			continue
		}
		switch action.Kind {
		case "ensure":
			ensured = true
		case "verify":
			verified = true
		}
	}
	if !ensured || !verified {
		t.Fatalf("identity plan actions missing: %#v", plan.Actions)
	}
}
