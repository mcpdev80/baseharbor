package main

import (
	"encoding/json"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestNextOnboardingTargetNameUsesEnvironmentRuntimeSequence(t *testing.T) {
	cfg := deployment.Config{Targets: map[string]deployment.TargetDefinition{
		"dev-docker-1": {},
		"dev-docker-3": {},
		"local":        {},
	}}
	if got := nextOnboardingTargetName(cfg, "dev", "docker"); got != "dev-docker-2" {
		t.Fatalf("next target name = %q, want dev-docker-2", got)
	}
	if got := nextOnboardingTargetName(cfg, "", "podman"); got != "dev-podman-1" {
		t.Fatalf("empty environment default = %q, want dev-podman-1", got)
	}
}

func TestParseOnboardImportSupportsOptionalTargetAlias(t *testing.T) {
	path, target, err := parseOnboardImportArgs([]string{"invite.json", "--target-name", "dev-docker-1"})
	if err != nil || path != "invite.json" || target != "dev-docker-1" {
		t.Fatalf("parsed import args = %q, %q, %v", path, target, err)
	}
	path, target, err = parseOnboardImportArgs([]string{"--target-name=dev-docker-2", "invite.json"})
	if err != nil || path != "invite.json" || target != "dev-docker-2" {
		t.Fatalf("parsed reordered import args = %q, %q, %v", path, target, err)
	}
	if _, _, err := parseOnboardImportArgs([]string{"invite.json", "--target-name", "one", "--target-name", "two"}); err == nil {
		t.Fatal("duplicate Target aliases accepted")
	}
	for _, args := range [][]string{{"invite.json", "--target-name", ""}, {"invite.json", "--target-name="}, {"invite.json", "--target", "ignored"}} {
		if _, _, err := parseOnboardImportArgs(args); err == nil {
			t.Fatalf("empty Target alias accepted: %#v", args)
		}
	}
}

func TestImportTargetNameSurvivesGlobalOptionExtraction(t *testing.T) {
	args := []string{"onboard", "--import", "invite.json", "--target-name", "docker-vm-main"}
	filtered, _, _, globalTarget, err := extractGlobalOutputOptions(args)
	if err != nil || globalTarget != "" {
		t.Fatalf("import target name was consumed as a global target: %v (%v)", globalTarget, err)
	}
	if len(filtered) != len(args) {
		t.Fatalf("global option extraction changed import arguments: %#v", filtered)
	}
	if _, name, err := parseOnboardImportArgs(filtered[2:]); err != nil || name != "docker-vm-main" {
		t.Fatalf("custom alias did not reach importer: %q (%v)", name, err)
	}
}

func TestRemoteCoreMapsWorkstationTargetAliasToCoreSelector(t *testing.T) {
	core := &verifiedRemoteCore{target: "dev-docker-1", coreTarget: "local"}
	input, err := core.inputWithCoreTarget(json.RawMessage(`{"target":"dev-docker-1","application":"demo"}`))
	if err != nil || string(input) != `{"application":"demo","target":"local"}` {
		t.Fatalf("Core request target was not mapped: %s (%v)", input, err)
	}
	if _, err := core.inputWithCoreTarget(json.RawMessage(`{"target":"other"}`)); err == nil {
		t.Fatal("payload for a different Target was silently rewritten")
	}
	result, err := rewriteRemoteResultTarget(json.RawMessage(`{"target":"local","nested":{"target":"local"}}`), "local", "dev-docker-1")
	if err != nil || string(result) != `{"nested":{"target":"dev-docker-1"},"target":"dev-docker-1"}` {
		t.Fatalf("Core response Target was not presented through the workstation alias: %s (%v)", result, err)
	}
}
