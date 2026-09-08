package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestParseCreateArgsSupportsRequiredSecrets(t *testing.T) {
	name, environment, postgres, redis, secrets, required, err := parseCreateArgs([]string{
		"mailflow",
		"--environment", "production",
		"--postgres",
		"--redis",
		"--require-secret", "OPENAI_API_KEY",
		"--require-secret=SMTP_PASSWORD",
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "mailflow" || environment != "production" || !postgres || !redis || !secrets {
		t.Fatalf("unexpected create parse result: %q %q %t %t %t", name, environment, postgres, redis, secrets)
	}
	if !reflect.DeepEqual(required, []string{"OPENAI_API_KEY", "SMTP_PASSWORD"}) {
		t.Fatalf("unexpected required secrets %#v", required)
	}
}

func TestRequiredSecretsAppearInPlan(t *testing.T) {
	m := application.WithRequiredSecrets(application.New("mailflow", "production", true, true, true), "OPENAI_API_KEY", "SMTP_PASSWORD")
	plan, err := application.BuildPlan(m)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, action := range plan.Actions {
		joined += action.Kind + " " + action.Resource + " " + action.Description + "\n"
	}
	for _, name := range []string{"OPENAI_API_KEY", "SMTP_PASSWORD"} {
		if !strings.Contains(joined, "verify secret:"+name) {
			t.Fatalf("plan does not contain required secret %s: %s", name, joined)
		}
	}
}
