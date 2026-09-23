package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestParseCreateArgsSupportsRequiredSecrets(t *testing.T) {
	name, environment, postgres, redis, objectStorage, secrets, postgresInstances, redisInstances, objectStorageBuckets, required, err := parseCreateArgs([]string{
		"mailflow",
		"--environment", "production",
		"--sql",
		"--cache",
		"--require-secret", "OPENAI_API_KEY",
		"--require-secret=SMTP_PASSWORD",
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "mailflow" || environment != "production" || !postgres || !redis || objectStorage || !secrets {
		t.Fatalf("unexpected create parse result: %q %q %t %t %t", name, environment, postgres, redis, secrets)
	}
	if len(postgresInstances) != 0 || len(redisInstances) != 0 || len(objectStorageBuckets) != 0 {
		t.Fatalf("unexpected named service instances: postgres=%#v redis=%#v s3=%#v", postgresInstances, redisInstances, objectStorageBuckets)
	}
	if !reflect.DeepEqual(required, []string{"OPENAI_API_KEY", "SMTP_PASSWORD"}) {
		t.Fatalf("unexpected required secrets %#v", required)
	}
}

func TestParseCreateArgsSupportsNamedServiceInstances(t *testing.T) {
	_, _, postgres, redis, objectStorage, _, postgresInstances, redisInstances, objectStorageBuckets, _, err := parseCreateArgs([]string{
		"mailflow",
		"--sql-instance", "primary",
		"--sql-instance=analytics",
		"--cache-instance", "cache",
		"--cache-instance=sessions",
	})
	if err != nil {
		t.Fatal(err)
	}
	if postgres || redis || objectStorage {
		t.Fatal("named instances must not implicitly request an additional default instance")
	}
	if len(objectStorageBuckets) != 0 {
		t.Fatalf("unexpected S3 buckets %#v", objectStorageBuckets)
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

func TestParseCreateArgsSupportsObjectStorageBuckets(t *testing.T) {
	name, environment, postgres, redis, objectStorage, secrets, postgresInstances, redisInstances, objectStorageBuckets, required, err := parseCreateArgs([]string{
		"assets-api",
		"--environment", "production",
		"--s3",
		"--s3-bucket", "uploads",
		"--s3-bucket=exports",
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "assets-api" || environment != "production" || postgres || redis || !objectStorage || secrets {
		t.Fatalf("unexpected create parse result: %q %q %t %t %t %t", name, environment, postgres, redis, objectStorage, secrets)
	}
	if len(postgresInstances) != 0 || len(redisInstances) != 0 || len(required) != 0 {
		t.Fatalf("unexpected unrelated values: postgres=%#v redis=%#v required=%#v", postgresInstances, redisInstances, required)
	}
	if !reflect.DeepEqual(objectStorageBuckets, []string{"uploads", "exports"}) {
		t.Fatalf("unexpected S3 buckets %#v", objectStorageBuckets)
	}
}

func TestManifestFromCreateArgsAllowsS3OnlyWithoutImplicitPostgres(t *testing.T) {
	m, err := manifestFromCreateArgs([]string{"assets-api", "--s3-bucket", "uploads"})
	if err != nil {
		t.Fatal(err)
	}
	if len(application.SQLInstanceNames(m)) != 0 {
		t.Fatalf("S3-only manifest unexpectedly includes PostgreSQL: %#v", m)
	}
	if !reflect.DeepEqual(application.ObjectStorageBucketNames(m), []string{"uploads"}) {
		t.Fatalf("unexpected S3 buckets %#v", application.ObjectStorageBucketNames(m))
	}
}
