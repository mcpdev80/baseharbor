package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// runManagedBackendConsole streams a client inside the application-owned
// backend container. No client binary is required on the developer device.
// Credentials are prepended on stdin, never inserted into process arguments.
func runManagedBackendConsole(ctx context.Context, resolved resolvedApplication, binding application.ServiceBinding, kind string, out, errOut io.Writer) error {
	if kind == "postgres" && application.UsesSharedPostgreSQL(resolved.Manifest) {
		return usageError("shared PostgreSQL requires a provider terminal capability", "Use a configured psql client for this shared binding until the shared-provider terminal is available.")
	}
	if kind == "valkey" && application.UsesSharedValkey(resolved.Manifest) {
		return usageError("shared Valkey requires a provider terminal capability", "Use a configured valkey-cli client for this shared binding until the shared-provider terminal is available.")
	}
	provider, err := detectRuntimeForApplication(ctx, resolved, bhruntime.CapabilityServiceExec)
	if err != nil {
		return fmt.Errorf("managed %s console is unavailable: %w", kind, err)
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return err
	}
	env, err := application.RuntimeEnvironment(files)
	if err != nil {
		return err
	}
	service, args, err := managedBackendCommand(kind, binding)
	if err != nil {
		return err
	}
	// Compose/Quadlet projects are isolated to the application's namespace.
	// The first stdin line contains the credential, consumed by the shell before
	// delegating the remaining input stream to the interactive client.
	input := io.MultiReader(strings.NewReader(binding.Password+"\n"), os.Stdin)
	fmt.Fprintf(errOut, "Opening managed %s console for %s/%s...\n", kind, resolved.Manifest.Name, binding.Instance)
	return provider.RunProjectFilesEnv(ctx, files.Project, files.Dir, env, input, out, errOut, []string{files.Compose}, append([]string{"exec", "-T", service}, args...)...)
}

func managedBackendCommand(kind string, binding application.ServiceBinding) (string, []string, error) {
	instance := strings.TrimSpace(binding.Instance)
	if instance == "" {
		return "", nil, fmt.Errorf("managed backend instance is required")
	}
	service := kind
	if instance != "default" {
		service += "-" + instance
	}
	switch kind {
	case "postgres":
		user := binding.Username
		if user == "" {
			user = "baseharbor"
		}
		db := binding.Database
		if db == "" {
			return "", nil, fmt.Errorf("managed postgres binding has no database")
		}
		script := "IFS= read -r PGPASSWORD || exit 2; export PGPASSWORD; export PGSSLMODE=require; exec psql -h 127.0.0.1 -p 5432 -U \"$1\" -d \"$2\""
		return service, []string{"sh", "-ec", script, "sh", user, db}, nil
	case "valkey":
		// Valkey is accessed over loopback inside its own managed container;
		// no password or unencrypted remote transport is exposed.
		script := "IFS= read -r REDISCLI_AUTH || exit 2; export REDISCLI_AUTH; exec valkey-cli -h 127.0.0.1 -p 6379"
		return service, []string{"sh", "-ec", script}, nil
	default:
		return "", nil, fmt.Errorf("unsupported managed backend console %q", kind)
	}
}
