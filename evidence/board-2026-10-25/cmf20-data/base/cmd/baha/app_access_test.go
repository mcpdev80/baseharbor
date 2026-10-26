package main

import "testing"

func TestSelectAccessInstance(t *testing.T) {
	got, err := selectAccessInstance([]string{"primary"}, "", "postgres")
	if err != nil || got != "primary" {
		t.Fatalf("single instance: got %q err=%v", got, err)
	}
	got, err = selectAccessInstance([]string{"primary", "analytics"}, "analytics", "postgres")
	if err != nil || got != "analytics" {
		t.Fatalf("named instance: got %q err=%v", got, err)
	}
	if _, err := selectAccessInstance([]string{"primary", "analytics"}, "", "postgres"); err == nil {
		t.Fatal("expected ambiguous instance selection to fail clearly")
	}
	if _, err := selectAccessInstance([]string{"primary"}, "missing", "postgres"); err == nil {
		t.Fatal("expected unknown instance to fail")
	}
}

func TestParseAccessTarget(t *testing.T) {
	appName, instance, err := parseAccessTarget([]string{"analytics", "--app", "mailflow"}, "psql")
	if err != nil {
		t.Fatal(err)
	}
	if appName != "mailflow" || instance != "analytics" {
		t.Fatalf("unexpected target %q/%q", appName, instance)
	}
}

func TestParseCredsArgsMasksByDefault(t *testing.T) {
	kind, appName, instance, reveal, err := parseCredsArgs([]string{"postgres", "primary", "--app=mailflow"})
	if err != nil {
		t.Fatal(err)
	}
	if kind != "postgres" || appName != "mailflow" || instance != "primary" || reveal {
		t.Fatalf("unexpected parse result")
	}
}

func TestParseLogsArgs(t *testing.T) {
	appName, service, follow, err := parseLogsArgs([]string{"api", "-f", "--app", "mailflow"})
	if err != nil {
		t.Fatal(err)
	}
	if appName != "mailflow" || service != "api" || !follow {
		t.Fatalf("unexpected log target")
	}
}
