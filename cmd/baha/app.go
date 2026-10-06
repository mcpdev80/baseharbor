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
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
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
	if err := application.ValidateApplicationID(m.ApplicationID); err != nil {
		return "", err
	}
	if err := m.Validate(); err != nil {
		return "", err
	}
	if err := authorizeMCPOperation(ctx, "app.create", "", m.Environment, m.ApplicationID, ""); err != nil {
		return "", err
	}
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
		Usage:   "baha app init [NAME] [-e ENV|--environment ENV] [capability options] [--workload-component NAME]... [--workload-source compose|quadlet|kubernetes:PATH]",
		Long:    "Creates baseharbor.yaml in the current directory for committing with the application source. The interactive capability picker uses detected defaults and lets you confirm them with a terminal checkbox UI; flags provide the deterministic non-interactive path for scripts and CI.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app init")
			if err != nil {
				return err
			}
			args = filtered
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
					"Use 'baha app init --quick' for repository detection, or pass explicit capability flags or --workload-component/--workload-source.",
				)
			}
			sourceSelection, sourceExplicit, err := parseWorkloadSourceArg(prepared)
			if err != nil {
				return err
			}
			persistSourceSelection := false
			if sourceExplicit {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				inspection, err := repositoryinspect.Inspect(ctx, cwd)
				if err != nil {
					return fmt.Errorf("inspect explicit workload source: %w", err)
				}
				selected, err := selectExplicitWorkloadSource(inspection.WorkloadSourceCandidates, sourceSelection)
				if err != nil {
					return err
				}
				autoSelected := inspection.WorkloadSourceResolution.Selected
				persistSourceSelection = autoSelected == nil ||
					autoSelected.Kind != sourceSelection.Kind ||
					filepath.ToSlash(autoSelected.Path) != filepath.ToSlash(sourceSelection.Path)
				if !hasExplicitWorkloadComponents(prepared) {
					evidence, err := repositoryinspect.NormalizeRepositoryWorkloadSource(cwd, selected)
					if err != nil {
						return fmt.Errorf("normalize explicit workload source: %w", err)
					}
					for _, component := range evidence.Components {
						if component.InfrastructureClass == "" {
							prepared = append(prepared, "--workload-component="+component.ID)
						}
					}
				}
			}
			if !sourceExplicit && hasExplicitWorkloadComponents(prepared) {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				inspection, err := repositoryinspect.Inspect(ctx, cwd)
				if err != nil {
					return fmt.Errorf("inspect repository workload source: %w", err)
				}
				if len(inspection.WorkloadSourceCandidates) > 1 && inspection.SelectedWorkloadSource == nil {
					return usageError(
						"multiple workload sources were detected but no explicit source was selected",
						"Pass --workload-source KIND:PATH using one of the sources shown by 'baha app inspect . --verbose'.",
					)
				}
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
				if len(detected.WorkloadSourceCandidates) > 0 || len(detected.WorkloadServices) > 0 {
					return usageError(
						"deterministic app init found repository workload evidence but no explicit workload selection",
						"Use 'baha app init --quick' for verified repository detection, or pass --workload-component and optionally --workload-source KIND:PATH explicitly.",
					)
				}
			}
			m, err := manifestFromCreateArgs(prepared)
			if err != nil {
				return err
			}
			var selected *repositoryinspect.WorkloadSourceCandidate
			if sourceExplicit && persistSourceSelection {
				selected = &sourceSelection
			}
			result, err := persistRepositoryApplication(ctx, ".", m, selected)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			absolute, repositoryMetadataPath := result.Manifest, result.SourceSelection
			fmt.Fprintf(out, "created repository manifest for %s (%s)\n", m.Name, m.Environment)
			fmt.Fprintf(out, "manifest: %s\n", absolute)
			if repositoryMetadataPath != "" {
				fmt.Fprintf(out, "repository source selection: %s\n", repositoryMetadataPath)
			}
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
			arg == "--workload-component",
			strings.HasPrefix(arg, "--workload-component="),
			arg == "--workload-source",
			strings.HasPrefix(arg, "--workload-source="):
			return true
		}
	}
	return false
}

func hasExplicitWorkloadSelection(args []string) bool {
	for _, arg := range args {
		if arg == "--workload-component" ||
			strings.HasPrefix(arg, "--workload-component=") ||
			arg == "--workload-source" ||
			strings.HasPrefix(arg, "--workload-source=") {
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
			"--s3-bucket", "--require-secret", "--workload-component", "--workload-source":
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
	workloadComponents, err := parseCreateWorkloadComponents(args)
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
		!options.secrets &&
		len(workloadComponents) == 0 {
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
	if len(workloadComponents) > 0 {
		m = application.WithWorkloadComponents(m, workloadComponents...)
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
		case arg == "--workload-component" || arg == "--workload-source":
			if i+1 >= len(args) {
				return createManifestOptions{}, usageError(arg+" requires a value", "Run 'baha app init --help' for available options.")
			}
			i++
		case strings.HasPrefix(arg, "--workload-component=") || strings.HasPrefix(arg, "--workload-source="):
			// Parsed separately by workload-source/workload-component helpers.
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

func parseCreateWorkloadComponents(args []string) ([]string, error) {
	var components []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--workload-component":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return nil, usageError("--workload-component requires a logical component name", "Example: --workload-component api")
			}
			i++
			components = append(components, strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "--workload-component="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--workload-component="))
			if value == "" {
				return nil, usageError("--workload-component requires a logical component name", "Example: --workload-component api")
			}
			components = append(components, value)
		}
	}
	return uniqueSorted(components), nil
}

func hasExplicitWorkloadComponents(args []string) bool {
	for _, arg := range args {
		if arg == "--workload-component" || strings.HasPrefix(arg, "--workload-component=") {
			return true
		}
	}
	return false
}

func parseWorkloadSourceArg(args []string) (repositoryinspect.WorkloadSourceCandidate, bool, error) {
	var raw string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--workload-source":
			if i+1 >= len(args) {
				return repositoryinspect.WorkloadSourceCandidate{}, false, usageError("--workload-source requires KIND:PATH", "Example: --workload-source kubernetes:deploy/k8s")
			}
			i++
			if raw != "" {
				return repositoryinspect.WorkloadSourceCandidate{}, false, usageError("--workload-source may be specified only once", "Choose one authoritative repository workload source.")
			}
			raw = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--workload-source="):
			if raw != "" {
				return repositoryinspect.WorkloadSourceCandidate{}, false, usageError("--workload-source may be specified only once", "Choose one authoritative repository workload source.")
			}
			raw = strings.TrimSpace(strings.TrimPrefix(arg, "--workload-source="))
		}
	}
	if raw == "" {
		return repositoryinspect.WorkloadSourceCandidate{}, false, nil
	}
	kindText, path, ok := strings.Cut(raw, ":")
	if !ok || strings.TrimSpace(path) == "" {
		return repositoryinspect.WorkloadSourceCandidate{}, false, usageError("--workload-source requires KIND:PATH", "Supported kinds: compose, quadlet, kubernetes.")
	}
	kind := repositoryinspect.WorkloadSourceKind(strings.ToLower(strings.TrimSpace(kindText)))
	switch kind {
	case repositoryinspect.WorkloadSourceCompose, repositoryinspect.WorkloadSourceQuadlet, repositoryinspect.WorkloadSourceKubernetes:
	default:
		return repositoryinspect.WorkloadSourceCandidate{}, false, usageError("unsupported workload source kind "+string(kind), "Supported kinds: compose, quadlet, kubernetes.")
	}
	return repositoryinspect.WorkloadSourceCandidate{Kind: kind, Path: filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))}, true, nil
}

func selectExplicitWorkloadSource(candidates []repositoryinspect.WorkloadSourceCandidate, requested repositoryinspect.WorkloadSourceCandidate) (repositoryinspect.WorkloadSourceCandidate, error) {
	for _, candidate := range candidates {
		if candidate.Kind == requested.Kind && filepath.ToSlash(candidate.Path) == filepath.ToSlash(requested.Path) {
			return candidate, nil
		}
	}
	return repositoryinspect.WorkloadSourceCandidate{}, usageError(
		fmt.Sprintf("requested workload source %s:%s was not detected", requested.Kind, requested.Path),
		"Run 'baha app inspect . --verbose' and choose one of the detected workload sources.",
	)
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
