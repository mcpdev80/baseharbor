package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestTargetListShowsImplicitLocalTarget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	var out bytes.Buffer
	cmd := targetCommand()
	list := cmd.Children[0]
	if err := list.Run(context.Background(), nil, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"local", "docker", "implicit", "effective"} {
		if !strings.Contains(text, want) {
			t.Fatalf("target list missing %q: %s", want, text)
		}
	}
}

func TestCreateTargetDoesNotBecomeDefaultWithoutFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	var out bytes.Buffer
	if err := createTarget(
		context.Background(),
		[]string{"kudo", "--provider", "docker", "--access", "local-docker", "--reference", "local", "--scope", "default"},
		&out,
		&out,
	); err != nil {
		t.Fatal(err)
	}

	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultTarget != "" {
		t.Fatalf("default target = %q, want empty so implicit local remains effective", cfg.DefaultTarget)
	}
	resolved, err := cfg.ResolveTarget("", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Name != "local" {
		t.Fatalf("effective target = %q, want implicit local", resolved.Name)
	}
}

func TestTargetStructuredOutputFlags(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	cmd := targetCommand()

	for _, args := range [][]string{
		{"--json"},
		{"-o", "json"},
		{"--output", "json"},
		{"--output=json"},
		{"-ojson"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var out bytes.Buffer
			if err := cmd.Run(context.Background(), args, &out, &out); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			var payload map[string]any
			if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
				t.Fatalf("%v: invalid JSON: %v\n%s", args, err, out.String())
			}
			if payload["contract_version"] != "v1" {
				t.Fatalf("%v: contract_version=%v", args, payload["contract_version"])
			}
		})
	}
}

func TestTargetListAndShowStructuredOutput(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	cmd := targetCommand()
	list := cmd.Children[0]
	show := cmd.Children[1]

	for _, args := range [][]string{{"--json"}, {"-ojson"}, {"--output=json"}} {
		var out bytes.Buffer
		if err := list.Run(context.Background(), args, &out, &out); err != nil {
			t.Fatalf("list %v: %v", args, err)
		}
		var payload struct {
			ContractVersion string           `json:"contract_version"`
			Targets         []targetListItem `json:"targets"`
		}
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("list %v invalid JSON: %v\n%s", args, err, out.String())
		}
		if payload.ContractVersion != "v1" || len(payload.Targets) == 0 || payload.Targets[0].Name == "" {
			t.Fatalf("list %v unexpected payload: %#v", args, payload)
		}
	}

	for _, args := range [][]string{
		{"local", "--json"},
		{"--json", "local"},
		{"local", "-ojson"},
	} {
		var out bytes.Buffer
		if err := show.Run(context.Background(), args, &out, &out); err != nil {
			t.Fatalf("show %v: %v", args, err)
		}
		var payload targetShowResult
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("show %v invalid JSON: %v\n%s", args, err, out.String())
		}
		if payload.ContractVersion != "v1" || payload.Target.Name != "local" {
			t.Fatalf("show %v unexpected payload: %#v", args, payload)
		}
	}

	var out bytes.Buffer
	if err := show.Run(context.Background(), []string{"--json"}, &out, &out); err != nil {
		t.Fatalf("show --json: %v", err)
	}
	var payload targetShowResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("show --json invalid JSON: %v\n%s", err, out.String())
	}
	if payload.Target.Name != "local" {
		t.Fatalf("show --json resolved %q, want local", payload.Target.Name)
	}
}
