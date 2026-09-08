package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestParseCreateArgsSupportsRequiredSecrets(t *testing.T) {
	name, environment, postgres, redis, secrets, postgresInstances, redisInstances, required, err := parseCreateArgs([]string{
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
	if len(postgresInstances) != 0 || len(redisInstances) != 0 {
		t.Fatalf("unexpected named service instances: postgres=%#v redis=%#v", postgresInstances, redisInstances)
	}
	if !reflect.DeepEqual(required, []string{"OPENAI_API_KEY", "SMTP_PASSWORD"}) {
		t.Fatalf("unexpected required secrets %#v", required)
	}
}

func TestParseCreateArgsSupportsNamedServiceInstances(t *testing.T) {
	_, _, postgres, redis, _, postgresInstances, redisInstances, _, err := parseCreateArgs([]string{
		"mailflow",
		"--postgres-instance", "primary",
		"--postgres-instance=analytics",
		"--redis-instance", "cache",
		"--redis-instance=sessions",
	})
	if err != nil {
		t.Fatal(err)
	}
	if postgres || redis {
		t.Fatal("named instances must not implicitly request an additional default instance")
	}
	if !reflect.DeepEqual(postgresInstances, []string{"primary", "analytics"}) {
		t.Fatalf("unexpected PostgreSQL instances %#v", postgresInstances)
	}
	if !reflect.DeepEqual(redisInstances, []string{"cache", "sessions"}) {
		t.Fatalf("unexpected Redis instances %#v", redisInstances)
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
