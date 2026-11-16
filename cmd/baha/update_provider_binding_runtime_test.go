package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type providerStopArgsRuntime struct {
	bhruntime.RuntimeProvider
	services []string
	compose  []string
}

func (f *providerStopArgsRuntime) StopProjectFilesSelected(_ context.Context, _, _ string, _ map[string]string, services []string, compose ...string) error {
	f.services = append([]string(nil), services...)
	f.compose = append([]string(nil), compose...)
	return nil
}

func TestProviderStopSelectedUsesServiceArgumentBeforeCompose(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "core.env")
	if err := os.WriteFile(env, []byte("CORE=owned\n"), 0600); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{Project: "owned", Compose: filepath.Join(dir, "core.yaml"), Env: env}
	rt := &providerStopArgsRuntime{}
	ops := &coreNativeRuntimeOps{runtime: rt}
	want := []string{"openbao-member-1", "openbao-member-2"}
	if err := ops.stopSelected(context.Background(), files, want...); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rt.services, want) || !reflect.DeepEqual(rt.compose, []string{files.Compose}) {
		t.Fatalf("incorrect native stop arguments: services=%v compose=%v", rt.services, rt.compose)
	}
}
