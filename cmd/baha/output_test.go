package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestParseReadOutputArgs(t *testing.T) {
	args, format, err := parseReadOutputArgs([]string{"demo", "-o", "json"}, "app plan")
	if err != nil {
		t.Fatal(err)
	}
	if format != outputJSON || len(args) != 1 || args[0] != "demo" {
		t.Fatalf("args=%v format=%q", args, format)
	}

	args, format, err = parseReadOutputArgs([]string{"--output=human"}, "app plan")
	if err != nil {
		t.Fatal(err)
	}
	if format != outputHuman || len(args) != 0 {
		t.Fatalf("args=%v format=%q", args, format)
	}
}

func TestParseReadOutputArgsRejectsUnsupportedFormat(t *testing.T) {
	if _, _, err := parseReadOutputArgs([]string{"-o", "yaml"}, "app plan"); err == nil {
		t.Fatal("expected unsupported output format to fail")
	}
}

func TestWriteJSONUsesStructuredPlanFields(t *testing.T) {
	plan := application.Plan{
		Application: "demo",
		Environment: "dev",
		Actions: []application.Action{
			{Kind: "ensure", Resource: "network", Description: "ready"},
		},
	}
	var out bytes.Buffer
	if err := writeJSON(&out, plan); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["application"] != "demo" || decoded["environment"] != "dev" {
		t.Fatalf("unexpected JSON: %s", out.String())
	}
}

func TestRequestsJSONOutput(t *testing.T) {
	for _, args := range [][]string{{"-o", "json"}, {"--output", "json"}, {"--output=json"}, {"--json"}} {
		if !requestsJSONOutput(args) {
			t.Fatalf("expected JSON detection for %v", args)
		}
	}
}
