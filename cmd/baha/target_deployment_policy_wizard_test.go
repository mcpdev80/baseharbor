package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestGuidedTargetDeploymentPolicyPresets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  string
		preset string
	}{
		{"development default", "\n\n", "development"},
		{"test", "test\nn\n", "test"},
		{"staging", "staging\nn\n", "staging"},
		{"production", "production\nn\n", "production"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			args, err := guidedTargetDeploymentPolicy(bufio.NewReader(strings.NewReader(tc.input)), &out)
			if err != nil {
				t.Fatal(err)
			}
			if len(args) != 2 || args[0] != "--deployment-preset" || args[1] != tc.preset {
				t.Fatalf("wrong preset args: %v", args)
			}
		})
	}
}

func TestGuidedTargetDeploymentPolicyEditableProduction(t *testing.T) {
	var out bytes.Buffer
	input := "production\ny\nall\nyes\nno\nno\nno\nno\n"
	args, err := guidedTargetDeploymentPolicy(bufio.NewReader(strings.NewReader(input)), &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 14 {
		t.Fatalf("wrong override count: %v", args)
	}
	flags := map[string]string{}
	for i := 4; i < len(args); i += 2 {
		flags[args[i]] = args[i+1]
	}
	p, err := resolveTargetDeploymentPolicyFlags(args[1], args[3], flags)
	if err != nil {
		t.Fatal(err)
	}
	if !p.AllowDirty || p.RequireCommit || p.RequireRemoteCommit || p.RequireApproval ||
		p.RequireImageDigest || !p.AllowsBranch("feature/dirty") {
		t.Fatalf("production overrides not applied: %+v", p)
	}
}

func TestGuidedTargetDeploymentPolicyNoneOrEOF(t *testing.T) {
	for _, input := range []string{"none\n", ""} {
		var out bytes.Buffer
		args, err := guidedTargetDeploymentPolicy(bufio.NewReader(strings.NewReader(input)), &out)
		if err != nil || args != nil {
			t.Fatalf("unexpected policy %v / %v", args, err)
		}
	}
}
