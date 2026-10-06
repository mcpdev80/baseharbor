package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

func TestOrganizationPreferencesCLIAndHTTPShareProvenanceAndDenial(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	digest := "sha256:" + strings.Repeat("a", 64)
	state := orgconfig.ActiveState{
		Resolution: orgconfig.Resolution{Source: orgconfig.Source{Kind: orgconfig.SourceLocal, Location: "organization.yaml"}, ResolvedDigest: digest},
		Config: orgconfig.Config{APIVersion: orgconfig.ContractVersion, Organization: "parity",
			Targets:     map[string]orgconfig.Reference{"company": {Reference: "company-target"}},
			Defaults:    orgconfig.EnvironmentDefaults{Target: "company"},
			Constraints: []orgconfig.Constraint{{Field: "target", Allowed: []string{"company", "permitted-target"}}},
		},
	}
	if err := orgconfig.SaveActive(state); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"permitted-target", "forbidden-target"} {
		preferences := []orgconfig.PreferenceLayer{{Scope: orgconfig.ScopeInvocation, Identity: "request", Defaults: orgconfig.EnvironmentDefaults{Target: target}}}
		data, _ := json.Marshal(preferences)
		file := filepath.Join(root, "preferences.json")
		if err := os.WriteFile(file, data, 0o600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		cliErr := runOrganizationShow(context.Background(), []string{"--preferences", file, "--environment", "dev", "-o", "json"}, &out, &bytes.Buffer{})
		input, _ := json.Marshal(machineOrganizationInput{Environment: "dev", Preferences: preferences})
		result, httpErr := executeHTTPOrganizationRead(context.Background(), "organization.inspect", machine.OperationContext{Environment: "dev"}, input)
		if target == "forbidden-target" {
			for _, err := range []error{cliErr, httpErr} {
				var problem *machine.Error
				if !errors.As(err, &problem) || problem.Code != machine.ErrorPolicyDenied || problem.Resource != "target" {
					t.Fatalf("denial lost across surface: %v", err)
				}
			}
			continue
		}
		if cliErr != nil || httpErr != nil {
			t.Fatalf("permitted override failed: CLI=%v HTTP=%v", cliErr, httpErr)
		}
		var cliView organizationView
		if err := json.Unmarshal(out.Bytes(), &cliView); err != nil {
			t.Fatal(err)
		}
		httpView := result.(organizationView)
		cliJSON, _ := json.Marshal(cliView.Effective)
		httpJSON, _ := json.Marshal(httpView.Effective)
		if !bytes.Equal(cliJSON, httpJSON) || cliView.Effective.Provenance["target"].Winner.Scope != orgconfig.ScopeInvocation {
			t.Fatal("effective value/policy/provenance diverged across CLI and HTTP")
		}
	}
}

func TestOrganizationTargetOverrideCannotBypassManagedConstraint(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("BASEHARBOR_TARGET", "local")
	state := orgconfig.ActiveState{
		Resolution: orgconfig.Resolution{Source: orgconfig.Source{Kind: orgconfig.SourceLocal, Location: "organization.yaml"}},
		Config: orgconfig.Config{APIVersion: orgconfig.ContractVersion, Organization: "acme",
			Targets:     map[string]orgconfig.Reference{"company": {Reference: "local"}},
			Defaults:    orgconfig.EnvironmentDefaults{Target: "company"},
			Constraints: []orgconfig.Constraint{{Field: "target", Allowed: []string{"company"}}},
		},
	}
	if err := orgconfig.SaveActive(state); err != nil {
		t.Fatal(err)
	}
	if _, err := effectiveTarget(context.Background()); err != nil {
		t.Fatal("canonical reference equivalent to managed selection denied:", err)
	}
	_, err := effectiveTarget(withTargetOverride(context.Background(), "forbidden"))
	var problem *machine.Error
	if !errors.As(err, &problem) || problem.Code != machine.ErrorPolicyDenied {
		t.Fatalf("explicit target escaped mandatory policy: %v", err)
	}
}
