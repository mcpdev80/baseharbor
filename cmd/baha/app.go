package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func appCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "app",
		Summary: "Manage declarative application backend runtimes",
		Usage:   "baha app <command> [options]",
		Long:    "Applications are independent consumers of BaseHarbor. Put baseharbor.yaml in the application repository and run app commands without NAME, or pass NAME explicitly for compatibility with stored application state. Manifests contain desired backend services and required secret names, never plaintext credentials.",
		Children: []*cli.Command{
			appInitCommand(),
			appCreateCommand(),
			appListCommand(),
			appShowCommand(store),
			appPlanCommand(store),
			appPreflightCommand(store),
		},
	}
}

func createTargetManagedApplication(ctx context.Context, m application.Manifest) (string, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return "", err
	}
	id := deployment.DeploymentIdentity{
		Target:      target.Name,
		Application: m.Name,
		Environment: m.Environment,
	}
	root, err := deployment.DeploymentRoot(id)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(root); err == nil {
		return "", fmt.Errorf("%w: %s/%s/%s", application.ErrExists, id.Target, id.Application, id.Environment)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	sourceRoot := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(sourceRoot, application.RepositoryManifestName)
	if err := os.WriteFile(path, []byte(m.YAML()), 0o600); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	intent, err := json.Marshal(m)
	if err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: id,
		Source: deployment.DeploymentSource{
			Kind:       "managed",
			Repository: sourceRoot,
			Manifest:   path,
		},
		Applied: deployment.AppliedDeployment{
			Intent:          intent,
			RuntimeProvider: target.RuntimeProvider,
			GeneratedState: map[string]string{
				"deployment": root,
				"state":      filepath.Join(root, "state"),
			},
		},
		Observed: deployment.ObservedDeployment{State: "created"},
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	return path, nil
}

func appInitCommand() *cli.Command {
	return &cli.Command{
		Name:    "init",
		Summary: "Create a repository-owned baseharbor.yaml",
		Usage:   "baha app init [NAME] [-e ENV|--environment ENV] [--sql] [--sql-instance NAME]... [--cache] [--cache-instance NAME]... [--s3] [--s3-bucket NAME]... [--secrets] [--require-secret NAME]... [--workload-compose FILE --workload-service NAME]...",
		Long:    "Creates baseharbor.yaml in the current directory for committing with the application source. The interactive capability picker uses detected defaults and lets you confirm them with a terminal checkbox UI; flags provide the deterministic non-interactive path for scripts and CI.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			prepared := append([]string(nil), args...)
			if !hasCreateName(prepared) {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				prepared = append([]string{filepath.Base(cwd)}, prepared...)
			}
			m, err := manifestFromCreateArgs(prepared)
			if err != nil {
				return err
			}
			path := application.RepositoryManifestName
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				if errors.Is(err, os.ErrExist) {
					return fmt.Errorf("%s already exists; edit the existing application contract instead", path)
				}
				return err
			}
			if _, err := file.WriteString(m.YAML()); err != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			absolute, _ := filepath.Abs(path)
			fmt.Fprintf(out, "created repository manifest for %s (%s)\n", m.Name, m.Environment)
			fmt.Fprintf(out, "manifest: %s\n", absolute)
			fmt.Fprintln(out, "next: review baseharbor.yaml, commit it, then run 'baha up'")
			return nil
		},
	}
}

func hasCreateName(args []string) bool {
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		switch arg {
		case "--environment", "-e", "--sql-instance", "--cache-instance", "--s3-bucket", "--require-secret", "--workload-compose", "--workload-service":
			skipNext = true
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

func manifestFromCreateArgs(args []string) (application.Manifest, error) {
	name, environment, sql, cache, objectStorage, secrets, sqlInstances, cacheInstances, objectStorageBuckets, required, err := parseCreateArgs(args)
	if err != nil {
		return application.Manifest{}, err
	}
	workloadCompose, workloadServices, err := parseCreateWorkloadArgs(args)
	if err != nil {
		return application.Manifest{}, err
	}
	if !sql && len(sqlInstances) == 0 && !cache && len(cacheInstances) == 0 && !objectStorage && len(objectStorageBuckets) == 0 && !secrets {
		sql = true
	}
	m := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        name,
		Environment: environment,
		Services: application.Services{
			SQL:           sql || len(sqlInstances) > 0,
			Cache:         cache || len(cacheInstances) > 0,
			ObjectStorage: objectStorage || len(objectStorageBuckets) > 0,
			Secrets:       secrets,
		},
	}
	if len(sqlInstances) > 0 {
		if sql {
			sqlInstances = append(sqlInstances, "default")
		}
		m = application.WithSQLInstances(m, sqlInstances...)
	}
	if len(cacheInstances) > 0 {
		if cache {
			cacheInstances = append(cacheInstances, "default")
		}
		m = application.WithCacheInstances(m, cacheInstances...)
	}
	if len(objectStorageBuckets) > 0 {
		if objectStorage {
			objectStorageBuckets = append(objectStorageBuckets, "default")
		}
		m = application.WithObjectStorageBuckets(m, objectStorageBuckets...)
	}
	m = application.WithRequiredSecrets(m, required...)
	if workloadCompose != "" {
		m = application.WithWorkload(m, workloadCompose, workloadServices...)
	}
	if err := m.Validate(); err != nil {
		return application.Manifest{}, err
	}
	return m, nil
}

func parseCreateArgs(args []string) (name, environment string, sql, cache, objectStorage, secrets bool, sqlInstances, cacheInstances, objectStorageBuckets, required []string, err error) {
	environment = "dev"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--sql":
			sql = true
		case arg == "--sql-instance":
			if i+1 >= len(args) {
				return "", "", false, false, false, false, nil, nil, nil, nil, usageError("--sql-instance requires a name", "Example: --sql-instance analytics")
			}
			i++
			sqlInstances = append(sqlInstances, args[i])
		case strings.HasPrefix(arg, "--sql-instance="):
			sqlInstances = append(sqlInstances, strings.TrimPrefix(arg, "--sql-instance="))
		case arg == "--cache":
			cache = true
		case arg == "--cache-instance":
			if i+1 >= len(args) {
				return "", "", false, false, false, false, nil, nil, nil, nil, usageError("--cache-instance requires a name", "Example: --cache-instance sessions")
			}
			i++
			cacheInstances = append(cacheInstances, args[i])
		case strings.HasPrefix(arg, "--cache-instance="):
			cacheInstances = append(cacheInstances, strings.TrimPrefix(arg, "--cache-instance="))
		case arg == "--s3":
			objectStorage = true
		case arg == "--s3-bucket":
			if i+1 >= len(args) {
				return "", "", false, false, false, false, nil, nil, nil, nil, usageError("--s3-bucket requires a name", "Example: --s3-bucket assets")
			}
			i++
			objectStorageBuckets = append(objectStorageBuckets, args[i])
		case strings.HasPrefix(arg, "--s3-bucket="):
			objectStorageBuckets = append(objectStorageBuckets, strings.TrimPrefix(arg, "--s3-bucket="))
		case arg == "--secrets":
			secrets = true
		case arg == "--require-secret":
			if i+1 >= len(args) {
				return "", "", false, false, false, false, nil, nil, nil, nil, usageError("--require-secret requires a name", "Example: --require-secret OPENAI_API_KEY")
			}
			i++
			required = append(required, args[i])
			secrets = true
		case strings.HasPrefix(arg, "--require-secret="):
			required = append(required, strings.TrimPrefix(arg, "--require-secret="))
			secrets = true
		case arg == "--environment" || arg == "-e":
			if i+1 >= len(args) {
				return "", "", false, false, false, false, nil, nil, nil, nil, usageError("--environment requires a value", "Example: --environment prod")
			}
			i++
			environment = args[i]
		case strings.HasPrefix(arg, "--environment="):
			environment = strings.TrimPrefix(arg, "--environment=")
		case arg == "--workload-compose" || arg == "--workload-service":
			if i+1 >= len(args) {
				return "", "", false, false, false, false, nil, nil, nil, nil, usageError(arg+" requires a value", "Run 'baha app init --help' for available options.")
			}
			i++
		case strings.HasPrefix(arg, "--workload-compose=") || strings.HasPrefix(arg, "--workload-service="):
			// Parsed separately by parseCreateWorkloadArgs.
		case strings.HasPrefix(arg, "-"):
			return "", "", false, false, false, false, nil, nil, nil, nil, usageError("unknown option "+arg, "Run 'baha app create --help' for available options.")
		default:
			if name != "" {
				return "", "", false, false, false, false, nil, nil, nil, nil, usageError("application manifest generation accepts exactly one NAME", "Example: baha app init demo --sql")
			}
			name = arg
		}
	}
	if name == "" {
		return "", "", false, false, false, false, nil, nil, nil, nil, usageError("application name is required", "Pass NAME or run 'baha app init' from a directory whose name is a valid application slug.")
	}
	return name, environment, sql, cache, objectStorage, secrets, sqlInstances, cacheInstances, objectStorageBuckets, required, nil
}

func parseCreateWorkloadArgs(args []string) (string, []string, error) {
	var compose string
	var services []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--workload-compose":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", nil, usageError("--workload-compose requires a file path", "Example: --workload-compose compose.yaml")
			}
			i++
			compose = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--workload-compose="):
			compose = strings.TrimSpace(strings.TrimPrefix(arg, "--workload-compose="))
			if compose == "" {
				return "", nil, usageError("--workload-compose requires a file path", "Example: --workload-compose compose.yaml")
			}
		case arg == "--workload-service":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", nil, usageError("--workload-service requires a service name", "Example: --workload-service api")
			}
			i++
			services = append(services, strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "--workload-service="):
			service := strings.TrimSpace(strings.TrimPrefix(arg, "--workload-service="))
			if service == "" {
				return "", nil, usageError("--workload-service requires a service name", "Example: --workload-service api")
			}
			services = append(services, service)
		}
	}
	services = uniqueSorted(services)
	switch {
	case compose == "" && len(services) > 0:
		return "", nil, usageError("--workload-service requires --workload-compose", "Example: --workload-compose compose.yaml --workload-service api")
	case compose != "" && len(services) == 0:
		return "", nil, usageError("--workload-compose requires at least one --workload-service", "Example: --workload-compose compose.yaml --workload-service api")
	}
	return compose, services, nil
}

func serviceNames(m application.Manifest) string {
	var names []string
	if count := len(application.SQLInstanceNames(m)); count > 0 {
		if count == 1 {
			names = append(names, "sql")
		} else {
			names = append(names, fmt.Sprintf("sql(%d)", count))
		}
	}
	if count := len(application.CacheInstanceNames(m)); count > 0 {
		if count == 1 {
			names = append(names, "cache")
		} else {
			names = append(names, fmt.Sprintf("cache(%d)", count))
		}
	}
	if count := len(application.ObjectStorageBucketNames(m)); count > 0 {
		if count == 1 {
			names = append(names, "s3")
		} else {
			names = append(names, fmt.Sprintf("s3(%d)", count))
		}
	}
	if application.HasOTLPTelemetry(m) {
		names = append(names, "otlp")
	}
	if m.Services.Secrets {
		names = append(names, "secrets")
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}
