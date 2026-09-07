package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"init"}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	path := filepath.Join(dir, "baseharbor.yaml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected permissions: %o", info.Mode().Perm())
	}
}

func TestRootAndNestedHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"app", "--help"}, {"app", "create", "--help"}} {
		var out bytes.Buffer
		if err := runWithIO(context.Background(), args, &out, &out); err != nil {
			t.Fatalf("help %v failed: %v", args, err)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Fatalf("help %v missing usage: %s", args, out.String())
		}
	}
}

func TestAppCreateListShowPlan(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "create", "demo", "--environment", "test", "--postgres", "--redis"}, &out, &out); err != nil {
		t.Fatalf("create failed: %v\n%s", err, out.String())
	}
	manifest := filepath.Join(dir, ".baseharbor", "apps", "demo", "baseharbor.yaml")
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}

	out.Reset()
	if err := runWithIO(context.Background(), []string{"app", "list"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "demo") || !strings.Contains(out.String(), "postgres,redis") {
		t.Fatalf("unexpected list: %s", out.String())
	}

	out.Reset()
	if err := runWithIO(context.Background(), []string{"app", "show", "demo"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "environment: test") {
		t.Fatalf("unexpected show: %s", out.String())
	}

	out.Reset()
	if err := runWithIO(context.Background(), []string{"app", "plan", "demo"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ensure postgres") || !strings.Contains(out.String(), "No changes were made") {
		t.Fatalf("unexpected plan: %s", out.String())
	}
}

func TestUnknownCommandFails(t *testing.T) {
	if err := run([]string{"does-not-exist"}); err == nil {
		t.Fatal("expected unknown command to fail")
	}
}
