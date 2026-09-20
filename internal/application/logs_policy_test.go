package application

import "testing"

func TestLogsCollectionPolicyDefaults(t *testing.T) {
	dev := New("demo", "dev", false, false, false)
	enabled, err := LogsCollectionEnabled(dev)
	if err != nil || !enabled {
		t.Fatalf("dev enabled=%t err=%v", enabled, err)
	}
	prod := dev
	prod.Environment = "production"
	enabled, err = LogsCollectionEnabled(prod)
	if err != nil || enabled {
		t.Fatalf("production enabled=%t err=%v", enabled, err)
	}
}

func TestLogsPolicyExplicitSourceClasses(t *testing.T) {
	t.Setenv(LogsEnabledEnv, "true")
	t.Setenv(LogsCollectSourcesEnv, "application,application-provider")
	policy, err := LogsPolicy(New("demo", "production", false, false, false))
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Enabled || !policy.Collect[LogsSourceApplication] || !policy.Collect[LogsSourceApplicationProvider] || policy.Collect[LogsSourcePlatformProvider] {
		t.Fatalf("policy=%#v", policy)
	}
}

func TestLogsPolicyRejectsUnknownSourceClass(t *testing.T) {
	t.Setenv(LogsCollectSourcesEnv, "application,everything")
	if _, err := LogsPolicy(New("demo", "dev", false, false, false)); err == nil {
		t.Fatal("unsupported logs source class accepted")
	}
}
