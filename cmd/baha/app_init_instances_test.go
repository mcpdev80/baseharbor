package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func TestManifestFromDetectedProjectPreservesMultipleLogicalInstances(t *testing.T) {
	d := appProjectDetection{
		Name:              "mailflow",
		Postgres:          true,
		PostgresInstances: []string{"primary", "analytics"},
		Redis:             true,
		RedisInstances:    []string{"cache", "sessions"},
	}
	m, err := manifestFromDetectedProject(d, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(application.PostgresInstanceNames(m), ","); got != "analytics,primary" {
		t.Fatalf("postgres instances = %q", got)
	}
	if got := strings.Join(application.RedisInstanceNames(m), ","); got != "cache,sessions" {
		t.Fatalf("redis instances = %q", got)
	}
}

func TestQuickNamedInstancesKeepsSingleServiceAsDefault(t *testing.T) {
	if got := quickNamedInstances([]string{"postgres"}); got != nil {
		t.Fatalf("single detected service should remain one default logical instance: %#v", got)
	}
}

func TestDetectedLogicalInstanceNameRemovesBackendPrefix(t *testing.T) {
	if got := detectedLogicalInstanceName("postgres-analytics", "postgres"); got != "analytics" {
		t.Fatalf("postgres logical name = %q", got)
	}
	if got := detectedLogicalInstanceName("redis-sessions", "redis"); got != "sessions" {
		t.Fatalf("redis logical name = %q", got)
	}
}

func TestPromptServiceInstancesUsesDetectedMultipleDefaults(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	var out bytes.Buffer
	got, err := promptServiceInstances(reader, &out, "PostgreSQL", []string{"primary", "analytics"})
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(got, ","); joined != "analytics,primary" {
		t.Fatalf("instances = %q", joined)
	}
	if !strings.Contains(out.String(), "analytics,primary") {
		t.Fatalf("prompt did not expose detected defaults: %s", out.String())
	}
}

func TestRequiredSecretStatusIsActionable(t *testing.T) {
	var out bytes.Buffer
	printRequiredSecretStatus(&out, []openbao.RequiredSecretStatus{{Name: "OPENAI_API_KEY"}})
	text := out.String()
	for _, want := range []string{
		"No application secrets have been configured yet.",
		"missing - user input required",
		"baha app secret set OPENAI_API_KEY --stdin",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}
