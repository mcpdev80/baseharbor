package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Native add/remove/re-add must read actual retained application data. An owned
// volume alone does not prove that either data or consumer credentials survived.
func qualifySelectedBackendData(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, resolved resolvedApplication, inventory application.SharedBackendInventory, service string, phase int, original *[32]byte) {
	t.Helper()
	kind, bindingKind := capability.ProviderPostgreSQL, "postgres"
	if service == "cache" {
		kind, bindingKind = capability.ProviderValkey, "valkey"
	}
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	binding, err := application.ResolveServiceBinding(files, bindingKind, "default")
	if err != nil {
		t.Fatal("actual application backend binding is unavailable")
	}
	identity := sha256.Sum256([]byte(binding.Username + "\x00" + binding.Database + "\x00" + binding.Password))
	if phase <= 0 {
		*original = identity
	} else if *original != identity {
		t.Fatal("backend re-add replaced retained application credentials or database identity")
	}
	root := filepath.Join(application.WorkloadServiceBindingProjectionDir(files), bindingKind)
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal("actual workload backend binding is unavailable")
		}
		return strings.TrimSpace(string(data))
	}
	host, port := read("host"), read("port")
	ca, err := os.ReadFile(binding.CertificatesPath)
	if err != nil {
		t.Fatal("actual application backend trust is unavailable")
	}
	services := inventory.Services[kind]
	if len(services) == 0 {
		t.Fatal("actual selected backend has no canonical service")
	}
	// Credentials and trust travel only over stdin. Provider error output may
	// contain connection data, so neither it nor the result enters diagnostics.
	input := append([]byte(binding.Password+"\n"), ca...)
	defer clear(input)
	const trustScript = "IFS= read -r password || exit 1\ntrust=$(mktemp /tmp/baha-retained-data-ca.XXXXXX) || exit 1\ntrap 'rm -f \"$trust\"' EXIT\ncat >\"$trust\"\n"
	marker := fmt.Sprintf("retained-%x", sha256.Sum256([]byte(resolved.Manifest.ApplicationID)))
	var result string
	if service == "sql" {
		query := "SELECT value FROM public.baha_retained_data_marker"
		if phase == -1 {
			query = "SELECT CASE WHEN to_regclass('public.baha_retained_data_marker') IS NULL THEN 'absent' ELSE 'present' END"
		}
		if phase == 0 {
			query = fmt.Sprintf("CREATE TABLE public.baha_retained_data_marker(value text NOT NULL); INSERT INTO public.baha_retained_data_marker VALUES ('%s'); SELECT value FROM public.baha_retained_data_marker", marker)
		}
		script := trustScript + "export PGPASSWORD=\"$password\" PGSSLMODE=verify-full PGSSLROOTCERT=\"$trust\" PGCONNECT_TIMEOUT=5\npsql --no-psqlrc -h \"$1\" -p \"$2\" -U \"$3\" -d \"$4\" -Atq -v ON_ERROR_STOP=1 -c \"$5\""
		result, err = provider.ExecProjectInput(ctx, inventory.Files.Project, inventory.Files.Compose, inventory.Files.Env, input, services[0], "sh", "-ec", script, "--", host, port, binding.Username, binding.Database, query)
	} else {
		key := read("baseharbor-key-prefix") + "baha_retained_data_marker"
		command := "GET"
		args := []string{host, port, binding.Username, command, key}
		if phase == 0 {
			args[3] = "SET"
			args = append(args, marker)
		}
		script := trustScript + "export VALKEYCLI_AUTH=\"$password\"\nvalkey-cli --no-auth-warning --tls --cacert \"$trust\" -h \"$1\" -p \"$2\" --user \"$3\" --raw \"$4\" \"$5\" ${6:+\"$6\"}"
		result, err = provider.ExecProjectInput(ctx, inventory.Files.Project, inventory.Files.Compose, inventory.Files.Env, input, services[0], append([]string{"sh", "-ec", script, "--"}, args...)...)
	}
	if err != nil {
		t.Fatal("actual consumer-authenticated backend data operation failed")
	}
	expected := marker
	if phase == -1 {
		expected = "absent"
		if service == "cache" {
			expected = ""
		}
	}
	if service == "cache" && phase == 0 {
		expected = "OK"
	}
	if strings.TrimSpace(result) != expected {
		t.Fatal("actual backend data did not survive removal and re-add")
	}
	if _, err := provider.ExecProjectInput(ctx, inventory.Files.Project, inventory.Files.Compose, inventory.Files.Env, nil, services[0], "sh", "-ec", `test -z "$(find /tmp -maxdepth 1 -name 'baha-retained-data-ca.*' -print -quit)"`); err != nil {
		t.Fatal("backend data probe retained temporary trust material")
	}
	t.Logf("backend=%s phase=%d actual consumer credential and TLS authenticated data assertion passed", service, phase)
}
