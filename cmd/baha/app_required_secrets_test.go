package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestParseCreateArgsSupportsRequiredSecrets(t *testing.T) {
	options, err := parseCreateArgs([]string{
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
	if options.name != "mailflow" || options.environment != "production" || !options.sql || !options.cache || options.objectStorage || !options.secrets {
		t.Fatalf("unexpected create parse result: %#v", options)
	}
	if len(options.sqlInstances) != 0 || len(options.cacheInstances) != 0 || len(options.objectStorageBuckets) != 0 {
		t.Fatalf("unexpected named service instances: %#v", options)
	}
	if !reflect.DeepEqual(options.requiredSecrets, []string{"OPENAI_API_KEY", "SMTP_PASSWORD"}) {
		t.Fatalf("unexpected required secrets %#v", options.requiredSecrets)
	}
}

func TestParseCreateArgsSupportsNamedServiceInstances(t *testing.T) {
	options, err := parseCreateArgs([]string{
		"mailflow",
		"--sql-instance", "primary",
		"--sql-instance=analytics",
		"--cache-instance", "cache",
		"--cache-instance=sessions",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.sql || options.cache || options.objectStorage {
		t.Fatal("named instances must not implicitly request an additional default instance")
	}
	if len(options.objectStorageBuckets) != 0 {
		t.Fatalf("unexpected S3 buckets %#v", options.objectStorageBuckets)
	}
	if !reflect.DeepEqual(options.sqlInstances, []string{"primary", "analytics"}) {
		t.Fatalf("unexpected PostgreSQL instances %#v", options.sqlInstances)
	}
	if !reflect.DeepEqual(options.cacheInstances, []string{"cache", "sessions"}) {
		t.Fatalf("unexpected Redis instances %#v", options.cacheInstances)
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
	options, err := parseCreateArgs([]string{
		"assets-api",
		"--environment", "production",
		"--s3",
		"--s3-bucket", "uploads",
		"--s3-bucket=exports",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.name != "assets-api" || options.environment != "production" || options.sql || options.cache || !options.objectStorage || options.secrets {
		t.Fatalf("unexpected create parse result: %#v", options)
	}
	if len(options.sqlInstances) != 0 || len(options.cacheInstances) != 0 || len(options.requiredSecrets) != 0 {
		t.Fatalf("unexpected unrelated values: %#v", options)
	}
	if !reflect.DeepEqual(options.objectStorageBuckets, []string{"uploads", "exports"}) {
		t.Fatalf("unexpected S3 buckets %#v", options.objectStorageBuckets)
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


func TestManifestFromCreateArgsSupportsAllV0419ServiceFamilies(t *testing.T) {
	m, err := manifestFromCreateArgs([]string{
		"platform",
		"--key-value-instance", "durable",
		"--document-db-instance", "documents",
		"--messaging-queue-instance", "jobs",
		"--messaging-pubsub-instance", "events",
		"--messaging-stream-instance", "audit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(application.KeyValueInstanceNames(m), []string{"durable"}) {
		t.Fatalf("durable key-value instances = %#v", application.KeyValueInstanceNames(m))
	}
	if !reflect.DeepEqual(application.DocumentDatabaseInstanceNames(m), []string{"documents"}) {
		t.Fatalf("document database instances = %#v", application.DocumentDatabaseInstanceNames(m))
	}
	if !reflect.DeepEqual(application.MessagingQueueInstanceNames(m), []string{"jobs"}) {
		t.Fatalf("messaging queue instances = %#v", application.MessagingQueueInstanceNames(m))
	}
	if !reflect.DeepEqual(application.MessagingPubSubInstanceNames(m), []string{"events"}) {
		t.Fatalf("messaging pubsub instances = %#v", application.MessagingPubSubInstanceNames(m))
	}
	if !reflect.DeepEqual(application.MessagingStreamInstanceNames(m), []string{"audit"}) {
		t.Fatalf("messaging stream instances = %#v", application.MessagingStreamInstanceNames(m))
	}
	if len(application.SQLInstanceNames(m)) != 0 {
		t.Fatalf("explicit v0.4.19 services unexpectedly added SQL: %#v", application.SQLInstanceNames(m))
	}
}
