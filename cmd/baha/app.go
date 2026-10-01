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
			appNewCommand(),
			appWorkspaceCommand(),
			appInitCommand(),
			appCreateCommand(),
			appListCommand(),
			appManifestShowCommand(store),
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
	if existing, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment); err != nil {
		return "", err
	} else if found {
		return "", fmt.Errorf("%w: %s/%s/%s [%s]", application.ErrExists, existing.Identity.Target, existing.Identity.Application, existing.Identity.Environment, existing.Identity.DeploymentID)
	}
	id, err := deployment.NewDeploymentIdentity(target.Name, m.ApplicationID, m.Name, m.Environment)
	if err != nil {
		return "", err
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
		Usage:   "baha app init [NAME] [-e ENV|--environment ENV] [--sql|--sql-instance NAME] [--cache|--cache-instance NAME] [--key-value|--key-value-instance NAME] [--document-db|--document-db-instance NAME] [--messaging-queue|--messaging-queue-instance NAME] [--messaging-pubsub|--messaging-pubsub-instance NAME] [--messaging-stream|--messaging-stream-instance NAME] [--s3|--s3-bucket NAME] [--secrets|--require-secret NAME] [--workload-compose FILE --workload-service NAME]...",
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
			if !hasExplicitInitContract(prepared) {
				return usageError(
					"deterministic app init requires an explicit capability or workload selection",
					"Use 'baha app init --quick' for repository detection, or pass explicit capability flags such as --sql, --cache, --key-value, --document-db, --messaging-queue, --s3, --secrets or --workload-compose/--workload-service.",
				)
			}
			if !hasExplicitWorkloadSelection(prepared) {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				detected, err := detectAppProject(cwd)
				if err != nil {
					return fmt.Errorf("inspect repository workload before deterministic init: %w", err)
				}
				if len(detected.ComposeCandidates) > 0 || strings.TrimSpace(detected.Compose) != "" || len(detected.WorkloadServices) > 0 {
					return usageError(
						"deterministic app init found repository workload evidence but no explicit workload selection",
						"Use 'baha app init --quick' for verified repository detection, or pass --workload-compose and --workload-service explicitly.",
					)
				}
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

func hasExplicitInitContract(args []string) bool {
	for _, arg := range args {
		switch {
		case arg == "--sql",
			arg == "--cache",
			arg == "--key-value",
			arg == "--document-db",
			arg == "--messaging-queue",
			arg == "--messaging-pubsub",
			arg == "--messaging-stream",
			arg == "--s3",
			arg == "--secrets",
			arg == "--sql-instance",
			strings.HasPrefix(arg, "--sql-instance="),
			arg == "--cache-instance",
			strings.HasPrefix(arg, "--cache-instance="),
			arg == "--key-value-instance",
			strings.HasPrefix(arg, "--key-value-instance="),
			arg == "--document-db-instance",
			strings.HasPrefix(arg, "--document-db-instance="),
			arg == "--messaging-queue-instance",
			strings.HasPrefix(arg, "--messaging-queue-instance="),
			arg == "--messaging-pubsub-instance",
			strings.HasPrefix(arg, "--messaging-pubsub-instance="),
			arg == "--messaging-stream-instance",
			strings.HasPrefix(arg, "--messaging-stream-instance="),
			arg == "--s3-bucket",
			strings.HasPrefix(arg, "--s3-bucket="),
			arg == "--require-secret",
			strings.HasPrefix(arg, "--require-secret="),
			arg == "--workload-compose",
			strings.HasPrefix(arg, "--workload-compose="),
			arg == "--workload-service",
			strings.HasPrefix(arg, "--workload-service="):
			return true
		}
	}
	return false
}

func hasExplicitWorkloadSelection(args []string) bool {
	for _, arg := range args {
		if arg == "--workload-compose" ||
			strings.HasPrefix(arg, "--workload-compose=") ||
			arg == "--workload-service" ||
			strings.HasPrefix(arg, "--workload-service=") {
			return true
		}
	}
	return false
}

func hasCreateName(args []string) bool {
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		switch arg {
		case "--environment", "-e",
			"--sql-instance", "--cache-instance", "--key-value-instance", "--document-db-instance",
			"--messaging-queue-instance", "--messaging-pubsub-instance", "--messaging-stream-instance",
			"--s3-bucket", "--require-secret", "--workload-compose", "--workload-service":
			skipNext = true
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

type createManifestOptions struct {
	name                      string
	environment               string
	sql                       bool
	cache                     bool
	keyValue                  bool
	documentDatabase          bool
	messagingQueue            bool
	messagingPubSub           bool
	messagingStream           bool
	objectStorage             bool
	secrets                   bool
	sqlInstances              []string
	cacheInstances            []string
	keyValueInstances         []string
	documentDatabaseInstances []string
	messagingQueueInstances   []string
	messagingPubSubInstances  []string
	messagingStreamInstances  []string
	objectStorageBuckets      []string
	requiredSecrets           []string
}

func manifestFromCreateArgs(args []string) (application.Manifest, error) {
	options, err := parseCreateArgs(args)
	if err != nil {
		return application.Manifest{}, err
	}
	workloadCompose, workloadServices, err := parseCreateWorkloadArgs(args)
	if err != nil {
		return application.Manifest{}, err
	}
	if !options.sql && len(options.sqlInstances) == 0 &&
		!options.cache && len(options.cacheInstances) == 0 &&
		!options.keyValue && len(options.keyValueInstances) == 0 &&
		!options.documentDatabase && len(options.documentDatabaseInstances) == 0 &&
		!options.messagingQueue && len(options.messagingQueueInstances) == 0 &&
		!options.messagingPubSub && len(options.messagingPubSubInstances) == 0 &&
		!options.messagingStream && len(options.messagingStreamInstances) == 0 &&
		!options.objectStorage && len(options.objectStorageBuckets) == 0 &&
		!options.secrets {
		options.sql = true
	}
	m := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          options.name,
		Environment:   options.environment,
		Services: application.Services{
			SQL:              options.sql || len(options.sqlInstances) > 0,
			Cache:            options.cache || len(options.cacheInstances) > 0,
			KeyValue:         options.keyValue || len(options.keyValueInstances) > 0,
			DocumentDatabase: options.documentDatabase || len(options.documentDatabaseInstances) > 0,
			MessagingQueue:   options.messagingQueue || len(options.messagingQueueInstances) > 0,
			MessagingPubSub:  options.messagingPubSub || len(options.messagingPubSubInstances) > 0,
			MessagingStream:  options.messagingStream || len(options.messagingStreamInstances) > 0,
			ObjectStorage:    options.objectStorage || len(options.objectStorageBuckets) > 0,
			Secrets:          options.secrets,
		},
	}
	if len(options.sqlInstances) > 0 {
		if options.sql {
			options.sqlInstances = append(options.sqlInstances, "default")
		}
		m = application.WithSQLInstances(m, options.sqlInstances...)
	}
	if len(options.cacheInstances) > 0 {
		if options.cache {
			options.cacheInstances = append(options.cacheInstances, "default")
		}
		m = application.WithCacheInstances(m, options.cacheInstances...)
	}
	if len(options.keyValueInstances) > 0 {
		if options.keyValue {
			options.keyValueInstances = append(options.keyValueInstances, "default")
		}
		m = application.WithKeyValueInstances(m, options.keyValueInstances...)
	}
	if len(options.documentDatabaseInstances) > 0 {
		if options.documentDatabase {
			options.documentDatabaseInstances = append(options.documentDatabaseInstances, "default")
		}
		m = application.WithDocumentDatabaseInstances(m, options.documentDatabaseInstances...)
	}
	if len(options.messagingQueueInstances) > 0 {
		if options.messagingQueue {
			options.messagingQueueInstances = append(options.messagingQueueInstances, "default")
		}
		m = application.WithMessagingQueueInstances(m, options.messagingQueueInstances...)
	}
	if len(options.messagingPubSubInstances) > 0 {
		if options.messagingPubSub {
			options.messagingPubSubInstances = append(options.messagingPubSubInstances, "default")
		}
		m = application.WithMessagingPubSubInstances(m, options.messagingPubSubInstances...)
	}
	if len(options.messagingStreamInstances) > 0 {
		if options.messagingStream {
			options.messagingStreamInstances = append(options.messagingStreamInstances, "default")
		}
		m = application.WithMessagingStreamInstances(m, options.messagingStreamInstances...)
	}
	if len(options.objectStorageBuckets) > 0 {
		if options.objectStorage {
			options.objectStorageBuckets = append(options.objectStorageBuckets, "default")
		}
		m = application.WithObjectStorageBuckets(m, options.objectStorageBuckets...)
	}
	m = application.WithRequiredSecrets(m, options.requiredSecrets...)
	if workloadCompose != "" {
		m = application.WithWorkload(m, workloadCompose, workloadServices...)
	}
	if err := m.Validate(); err != nil {
		return application.Manifest{}, err
	}
	return m, nil
}

func parseCreateArgs(args []string) (createManifestOptions, error) {
	options := createManifestOptions{environment: "dev"}
	nextValue := func(i *int, option, example string) (string, error) {
		if *i+1 >= len(args) || strings.TrimSpace(args[*i+1]) == "" {
			return "", usageError(option+" requires a name", "Example: "+example)
		}
		*i = *i + 1
		return args[*i], nil
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--sql":
			options.sql = true
		case arg == "--cache":
			options.cache = true
		case arg == "--key-value":
			options.keyValue = true
		case arg == "--document-db":
			options.documentDatabase = true
		case arg == "--messaging-queue":
			options.messagingQueue = true
		case arg == "--messaging-pubsub":
			options.messagingPubSub = true
		case arg == "--messaging-stream":
			options.messagingStream = true
		case arg == "--s3":
			options.objectStorage = true
		case arg == "--secrets":
			options.secrets = true
		case arg == "--sql-instance":
			value, err := nextValue(&i, arg, "--sql-instance analytics")
			if err != nil {
				return createManifestOptions{}, err
			}
			options.sqlInstances = append(options.sqlInstances, value)
		case strings.HasPrefix(arg, "--sql-instance="):
			options.sqlInstances = append(options.sqlInstances, strings.TrimPrefix(arg, "--sql-instance="))
		case arg == "--cache-instance":
			value, err := nextValue(&i, arg, "--cache-instance sessions")
			if err != nil {
				return createManifestOptions{}, err
			}
			options.cacheInstances = append(options.cacheInstances, value)
		case strings.HasPrefix(arg, "--cache-instance="):
			options.cacheInstances = append(options.cacheInstances, strings.TrimPrefix(arg, "--cache-instance="))
		case arg == "--key-value-instance":
			value, err := nextValue(&i, arg, "--key-value-instance durable")
			if err != nil {
				return createManifestOptions{}, err
			}
			options.keyValueInstances = append(options.keyValueInstances, value)
		case strings.HasPrefix(arg, "--key-value-instance="):
			options.keyValueInstances = append(options.keyValueInstances, strings.TrimPrefix(arg, "--key-value-instance="))
		case arg == "--document-db-instance":
			value, err := nextValue(&i, arg, "--document-db-instance documents")
			if err != nil {
				return createManifestOptions{}, err
			}
			options.documentDatabaseInstances = append(options.documentDatabaseInstances, value)
		case strings.HasPrefix(arg, "--document-db-instance="):
			options.documentDatabaseInstances = append(options.documentDatabaseInstances, strings.TrimPrefix(arg, "--document-db-instance="))
		case arg == "--messaging-queue-instance":
			value, err := nextValue(&i, arg, "--messaging-queue-instance jobs")
			if err != nil {
				return createManifestOptions{}, err
			}
			options.messagingQueueInstances = append(options.messagingQueueInstances, value)
		case strings.HasPrefix(arg, "--messaging-queue-instance="):
			options.messagingQueueInstances = append(options.messagingQueueInstances, strings.TrimPrefix(arg, "--messaging-queue-instance="))
		case arg == "--messaging-pubsub-instance":
			value, err := nextValue(&i, arg, "--messaging-pubsub-instance events")
			if err != nil {
				return createManifestOptions{}, err
			}
			options.messagingPubSubInstances = append(options.messagingPubSubInstances, value)
		case strings.HasPrefix(arg, "--messaging-pubsub-instance="):
			options.messagingPubSubInstances = append(options.messagingPubSubInstances, strings.TrimPrefix(arg, "--messaging-pubsub-instance="))
		case arg == "--messaging-stream-instance":
			value, err := nextValue(&i, arg, "--messaging-stream-instance audit")
			if err != nil {
				return createManifestOptions{}, err
			}
			options.messagingStreamInstances = append(options.messagingStreamInstances, value)
		case strings.HasPrefix(arg, "--messaging-stream-instance="):
			options.messagingStreamInstances = append(options.messagingStreamInstances, strings.TrimPrefix(arg, "--messaging-stream-instance="))
		case arg == "--s3-bucket":
			value, err := nextValue(&i, arg, "--s3-bucket assets")
			if err != nil {
				return createManifestOptions{}, err
			}
			options.objectStorageBuckets = append(options.objectStorageBuckets, value)
		case strings.HasPrefix(arg, "--s3-bucket="):
			options.objectStorageBuckets = append(options.objectStorageBuckets, strings.TrimPrefix(arg, "--s3-bucket="))
		case arg == "--require-secret":
			value, err := nextValue(&i, arg, "--require-secret OPENAI_API_KEY")
			if err != nil {
				return createManifestOptions{}, err
			}
			options.requiredSecrets = append(options.requiredSecrets, value)
			options.secrets = true
		case strings.HasPrefix(arg, "--require-secret="):
			options.requiredSecrets = append(options.requiredSecrets, strings.TrimPrefix(arg, "--require-secret="))
			options.secrets = true
		case arg == "--environment" || arg == "-e":
			if i+1 >= len(args) {
				return createManifestOptions{}, usageError("--environment requires a value", "Example: --environment prod")
			}
			i++
			options.environment = args[i]
		case strings.HasPrefix(arg, "--environment="):
			options.environment = strings.TrimPrefix(arg, "--environment=")
		case arg == "--workload-compose" || arg == "--workload-service":
			if i+1 >= len(args) {
				return createManifestOptions{}, usageError(arg+" requires a value", "Run 'baha app init --help' for available options.")
			}
			i++
		case strings.HasPrefix(arg, "--workload-compose=") || strings.HasPrefix(arg, "--workload-service="):
			// Parsed separately by parseCreateWorkloadArgs.
		case strings.HasPrefix(arg, "-"):
			return createManifestOptions{}, usageError("unknown option "+arg, "Run 'baha app init --help' for available options.")
		default:
			if options.name != "" {
				return createManifestOptions{}, usageError("application manifest generation accepts exactly one NAME", "Example: baha app init demo --sql")
			}
			options.name = arg
		}
	}
	if options.name == "" {
		return createManifestOptions{}, usageError("application name is required", "Pass NAME or run 'baha app init' from a directory whose name is a valid application slug.")
	}
	return options, nil
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
	if count := len(application.KeyValueInstanceNames(m)); count > 0 {
		if count == 1 {
			names = append(names, "key-value")
		} else {
			names = append(names, fmt.Sprintf("key-value(%d)", count))
		}
	}
	if count := len(application.DocumentDatabaseInstanceNames(m)); count > 0 {
		if count == 1 {
			names = append(names, "document-database")
		} else {
			names = append(names, fmt.Sprintf("document-database(%d)", count))
		}
	}
	if count := len(application.MessagingQueueInstanceNames(m)); count > 0 {
		if count == 1 {
			names = append(names, "messaging-queue")
		} else {
			names = append(names, fmt.Sprintf("messaging-queue(%d)", count))
		}
	}
	if count := len(application.MessagingPubSubInstanceNames(m)); count > 0 {
		if count == 1 {
			names = append(names, "messaging-pubsub")
		} else {
			names = append(names, fmt.Sprintf("messaging-pubsub(%d)", count))
		}
	}
	if count := len(application.MessagingStreamInstanceNames(m)); count > 0 {
		if count == 1 {
			names = append(names, "messaging-stream")
		} else {
			names = append(names, fmt.Sprintf("messaging-stream(%d)", count))
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
