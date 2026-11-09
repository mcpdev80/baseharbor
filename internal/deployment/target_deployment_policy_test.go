package deployment

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDeploymentPolicyPresetBestPractice(t *testing.T) {
	cases := []struct {
		name     string
		dirty    bool
		commit   bool
		remote   bool
		approval bool
		digest   bool
		allowed  []string
	}{
		{"development", true, false, false, false, false, nil},
		{"test", false, true, false, false, true, []string{"develop", "release/*"}},
		{"staging", false, true, false, false, true, []string{"main", "release/*"}},
		{"production", false, true, true, true, true, []string{"main"}},
		{"custom", false, false, false, false, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DeploymentPolicyPreset(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if got.AllowDirty != tc.dirty || got.RequireCommit != tc.commit ||
				got.RequireRemoteCommit != tc.remote || got.RequireApproval != tc.approval ||
				got.RequireImageDigest != tc.digest {
				t.Fatalf("wrong preset: %+v", got)
			}
			if len(got.AllowedBranches) != len(tc.allowed) {
				t.Fatalf("wrong branch list: %v", got.AllowedBranches)
			}
			for i, ref := range tc.allowed {
				if got.AllowedBranches[i] != ref {
					t.Fatalf("branch[%d]=%q want %q", i, got.AllowedBranches[i], ref)
				}
			}
			if err := got.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := DeploymentPolicyPreset("unknown"); err == nil {
		t.Fatal("unknown preset accepted")
	}
}

func TestDeploymentPolicyExplicitOverrides(t *testing.T) {
	p, err := DeploymentPolicyPreset("production")
	if err != nil {
		t.Fatal(err)
	}
	p.AllowDirty = true
	p.RequireCommit = false
	p.RequireRemoteCommit = false
	p.RequireApproval = false
	p.RequireImageDigest = false
	p.AllowedBranches = nil
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if !p.AllowDirty || !p.AllowsBranch("topic/unfinished") {
		t.Fatal("operator override ignored")
	}
}

func TestDeploymentPolicyBranches(t *testing.T) {
	p, _ := DeploymentPolicyPreset("test")
	if !p.AllowsBranch("develop") || !p.AllowsBranch("release/1.0") {
		t.Fatal("allowed branches denied")
	}
	if p.AllowsBranch("main") || p.AllowsBranch("feature/new") {
		t.Fatal("unlisted branch allowed")
	}
	for _, bad := range []string{"../main", "feature/**", "feature//x", "release/[", "", "/main", "main/"} {
		if err := (TargetDeploymentPolicy{AllowedBranches: []string{bad}}).Validate(); err == nil {
			t.Errorf("accepted bad branch pattern %q", bad)
		}
	}
}

func TestDeploymentPolicyOptionalConfigRoundTrip(t *testing.T) {
	var old struct {
		Name   string                  `yaml:"name"`
		Policy *TargetDeploymentPolicy `yaml:"deployment-policy,omitempty"`
	}
	if err := yaml.Unmarshal([]byte("name: previous\n"), &old); err != nil {
		t.Fatal(err)
	}
	if old.Policy != nil {
		t.Fatal("legacy target gained implicit restrictions")
	}
	p, _ := DeploymentPolicyPreset("production")
	data, err := yaml.Marshal(struct {
		Policy *TargetDeploymentPolicy `yaml:"deployment-policy,omitempty"`
	}{&p})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Policy *TargetDeploymentPolicy `yaml:"deployment-policy,omitempty"`
	}
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Policy == nil || !decoded.Policy.RequireApproval || decoded.Policy.AllowDirty {
		t.Fatalf("roundtrip changed policy: %+v", decoded.Policy)
	}
}
