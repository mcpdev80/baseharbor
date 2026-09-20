package application

import "testing"

func TestLogsCollectionPolicyDefaultsDisabledWithoutManifestIntent(t *testing.T) {
	dev := New("demo", "dev", false, false, false)
	enabled, err := LogsCollectionEnabled(dev)
	if err != nil || enabled {
		t.Fatalf("dev without logs intent enabled=%t err=%v", enabled, err)
	}
	prod := dev
	prod.Environment = "production"
	enabled, err = LogsCollectionEnabled(prod)
	if err != nil || enabled {
		t.Fatalf("production without logs intent enabled=%t err=%v", enabled, err)
	}
}

func TestLogsCollectionEnabledByManifestIntent(t *testing.T) {
	m := WithLogsCollection(New("demo", "dev", false, false, false), "application")
	m.Workload = WorkloadConfig{Compose: "compose.yaml", Services: []string{"api"}}
	enabled, err := LogsCollectionEnabled(m)
	if err != nil || !enabled {
		t.Fatalf("manifest logs intent enabled=%t err=%v", enabled, err)
	}
}

func TestLogsEnabledEnvCannotCreateUndeclaredIntent(t *testing.T) {
	t.Setenv(LogsEnabledEnv, "true")
	_, err := LogsCollectionEnabled(New("demo", "dev", false, false, false))
	if err == nil {
		t.Fatal("deployment override enabled undeclared logs intent")
	}
}

func TestLogsEnabledEnvCanDisableDeclaredIntent(t *testing.T) {
	t.Setenv(LogsEnabledEnv, "false")
	m := WithLogsCollection(New("demo", "dev", false, false, false), "application")
	m.Workload = WorkloadConfig{Compose: "compose.yaml", Services: []string{"api"}}
	enabled, err := LogsCollectionEnabled(m)
	if err != nil || enabled {
		t.Fatalf("declared logs intent was not disabled: enabled=%t err=%v", enabled, err)
	}
}

func TestLogsPolicyExplicitSourceClasses(t *testing.T) {
	t.Setenv(LogsCollectSourcesEnv, "application")
	m := WithLogsCollection(New("demo", "production", false, false, false), "application")
	m.Workload = WorkloadConfig{Compose: "compose.yaml", Services: []string{"api"}}
	policy, err := LogsPolicy(m)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Enabled || !policy.Collect[LogsSourceApplication] || policy.Collect[LogsSourceApplicationProvider] || policy.Collect[LogsSourcePlatformProvider] {
		t.Fatalf("policy=%#v", policy)
	}
}

func TestLogsPolicyRejectsUnknownSourceClass(t *testing.T) {
	t.Setenv(LogsCollectSourcesEnv, "application,everything")
	m := WithLogsCollection(New("demo", "dev", false, false, false), "application")
	m.Workload = WorkloadConfig{Compose: "compose.yaml", Services: []string{"api"}}
	if _, err := LogsPolicy(m); err == nil {
		t.Fatal("unsupported logs source class accepted")
	}
}
