package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

var appInitInput io.Reader = os.Stdin

type appProjectDetection struct {
	Name                   string
	ComposeCandidates      []string
	Compose                string
	WorkloadServices       []string
	InfrastructureServices []string
	AmbiguousServices      []string
	SQL                    bool
	SQLSource              string
	SQLInstances           []string
	Cache                  bool
	CacheSource            string
	CacheInstances         []string
	ObjectStorage          bool
	ObjectStorageSuggested bool
	ObjectStorageSource    string
	Metrics                bool
	MetricsSuggested       bool
	MetricsSource          string
	OTLP                   bool
	OTLPSuggested          bool
	OTLPSignals            []string
	OTLPSource             string
	LogsSuggested          bool
	RuntimeAPI             bool
	RuntimePermissions     map[string][]string
	Ports                  []repositoryinspect.PortEvidence
	SecretCandidates       []string
	SecretSources          map[string]string
	EnvFiles               []string
}

type guidedSecretPolicy struct {
	Name      string
	Source    string
	Required  bool
	Provision string
}

type composeServiceDetection struct {
	Name     string
	Postgres bool
	Redis    bool
	HasBuild bool
	HasImage bool
	HasPorts bool
}

func appGuidedInitCommand() *cli.Command {
	return &cli.Command{
		Name:    "init",
		Summary: "Detect the current project and create baseharbor.yaml",
		Usage:   "baha app init [--quick] | baha app init [NAME] [--environment ENV] [--sql] [--sql-instance NAME]... [--cache] [--cache-instance NAME]... [--s3] [--s3-bucket NAME]... [--secrets] [--require-secret NAME]...",
		Long:    "With no arguments, analyzes the current repository first and opens a compact guided setup that asks only about missing or ambiguous information. --quick accepts unambiguous detections and safe defaults without interactive questions. Existing flags keep the deterministic non-interactive manifest generator for CI and scripts.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			quick := false
			for _, arg := range args {
				if arg == "--quick" {
					quick = true
				}
			}
			if noInput(ctx) && len(args) == 0 {
				return usageError("app init needs explicit non-interactive input", "Use 'baha app init --quick' for detected safe defaults or provide deterministic app-init flags.")
			}
			if quick {
				if len(args) != 1 {
					return usageError("--quick cannot be combined with explicit app-init arguments", "Use either 'baha app init --quick' or the deterministic app-init flags.")
				}
			} else if len(args) > 0 {
				return appInitCommand().Run(ctx, args, out, errOut)
			}

			if _, err := os.Stat(application.RepositoryManifestName); err == nil {
				return fmt.Errorf("%s already exists; edit the existing application contract instead", application.RepositoryManifestName)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}

			detected, err := detectAppProject(".")
			if err != nil {
				return err
			}
			if quick {
				m, err := manifestFromDetectedProject(detected, true)
				if err != nil {
					return err
				}
				printProjectDetection(out, detected)
				fmt.Fprintln(out, "\nQuick mode selected detected values and safe defaults.")
				return writeRepositoryManifest(m, out)
			}

			if !appInitReaderIsTerminal(appInitInput) {
				return usageError("interactive app init requires a terminal", "Use 'baha app init --quick' for detected safe defaults or explicit flags for CI/scripts.")
			}
			return runAppInitWizard(ctx, detected, out)
		},
	}
}

func runAppInitWizard(ctx context.Context, d appProjectDetection, out io.Writer) error {
	reader := bufio.NewReader(appInitInput)
	fmt.Fprintln(out, "Analyzing repository...")
	printProjectDetection(out, d)
	fmt.Fprintln(out)

	name, err := promptLine(reader, out, "Application name", d.Name)
	if err != nil {
		return err
	}
	name = slugifyAppName(name)
	if name == "" {
		return errors.New("application name cannot be empty")
	}
	environment, err := promptLine(reader, out, "Environment", "dev")
	if err != nil {
		return err
	}
	environment = slugifyAppName(environment)
	if environment == "" {
		return errors.New("environment cannot be empty")
	}

	compose := d.Compose
	workloadServices := append([]string(nil), d.WorkloadServices...)
	ambiguousServices := append([]string(nil), d.AmbiguousServices...)
	if len(d.ComposeCandidates) > 1 {
		compose, err = promptCompose(reader, out, d.ComposeCandidates)
		if err != nil {
			return err
		}
		analysis, err := repositoryinspect.AnalyzeComposeFile(".", compose)
		if err != nil {
			return err
		}
		workloadServices = append([]string(nil), analysis.WorkloadServices...)
		ambiguousServices = append([]string(nil), analysis.AmbiguousServices...)
	}
	if len(ambiguousServices) > 0 {
		confirmedWorkload, err := promptAmbiguousComposeServices(reader, out, ambiguousServices)
		if err != nil {
			return err
		}
		workloadServices = uniqueSorted(append(workloadServices, confirmedWorkload...))
	}

	defaults := []bool{
		d.SQL,
		d.Cache,
		d.ObjectStorage,
		len(d.SecretCandidates) > 0,
		d.Metrics,
		d.OTLP && len(d.OTLPSignals) > 0,
		false,
	}
	allowNone := len(workloadServices) > 0
	selected, err := promptCapabilityList(reader, out, defaults, allowNone)
	if err != nil {
		return err
	}

	var sqlInstances, cacheInstances, objectStorageBuckets []string
	if selected[0] {
		sqlInstances, err = promptServiceInstances(reader, out, "PostgreSQL", d.SQLInstances)
		if err != nil {
			return err
		}
	}
	if selected[1] {
		cacheInstances, err = promptServiceInstances(reader, out, "Valkey / Redis", d.CacheInstances)
		if err != nil {
			return err
		}
	}
	if selected[2] {
		objectStorageBuckets, err = promptServiceInstances(reader, out, "S3 buckets", nil)
		if err != nil {
			return err
		}
		if len(objectStorageBuckets) == 0 {
			objectStorageBuckets = []string{"default"}
		}
	}

	var secretPolicies []guidedSecretPolicy
	if selected[3] {
		printManagedCredentialSummary(out, selected, len(d.RuntimePermissions) > 0)
		secretPolicies, err = promptSecretPolicies(reader, out, d.SecretCandidates, d.SecretSources)
		if err != nil {
			return err
		}
		additional, err := promptLine(reader, out, "Additional application secret names (comma-separated, Enter for none)", "")
		if err != nil {
			return err
		}
		for _, item := range strings.Split(additional, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				secretPolicies = append(secretPolicies, guidedSecretPolicy{Name: item, Required: true, Provision: "later"})
			}
		}
	}

	m := detectedApplicationManifest(name, environment, selected[0], selected[1], selected[2], selected[3], compose != "" && len(workloadServices) > 0)
	if len(sqlInstances) > 0 {
		m = application.WithSQLInstances(m, sqlInstances...)
	}
	if len(cacheInstances) > 0 {
		m = application.WithCacheInstances(m, cacheInstances...)
	}
	if len(objectStorageBuckets) > 0 {
		m = application.WithObjectStorageBuckets(m, objectStorageBuckets...)
	}
	m = applyGuidedSecretPolicies(m, secretPolicies)
	if compose != "" && len(workloadServices) > 0 {
		m = application.WithWorkload(m, filepath.ToSlash(compose), workloadServices...)
	}
	if selected[4] {
		service, port, ok := detectedMetricsTarget(d, workloadServices)
		if !ok {
			service, port, err = promptMetricsTarget(reader, out, workloadServices)
			if err != nil {
				return err
			}
		}
		m = application.WithMetricsSource(m, "application", service, port, "/metrics")
	}
	if selected[5] {
		defaultSignals := strings.Join(d.OTLPSignals, ",")
		if defaultSignals == "" {
			defaultSignals = "traces"
		}
		value, err := promptLine(reader, out, "OTLP signals (comma-separated)", defaultSignals)
		if err != nil {
			return err
		}
		signals := uniqueSorted(strings.Split(value, ","))
		if len(signals) == 0 {
			return errors.New("OTLP telemetry requires at least one signal")
		}
		m = application.WithOTLPTelemetry(m, signals...)
	}
	if selected[6] {
		m = application.WithLogsCollection(m, "application")
	}
	if len(d.RuntimePermissions) > 0 {
		services, err := runtimePermissionServices(reader, out, workloadServices)
		if err != nil {
			return err
		}
		for capabilityID, operations := range d.RuntimePermissions {
			m = application.WithRuntimePermission(m, capabilityID, services, operations...)
		}
	}
	if err := m.Validate(); err != nil {
		return err
	}

	printAdoptionSummary(out, m, d, secretPolicies)
	if cli.OutputOptionsFromContext(ctx).Verbose {
		fmt.Fprintln(out, "\nGenerated baseharbor.yaml")
		fmt.Fprintln(out, "----------------------------------------")
		fmt.Fprint(out, m.YAML())
		fmt.Fprintln(out, "----------------------------------------")
	}
	confirm, err := promptYesNo(reader, out, "Write baseharbor.yaml?", true)
	if err != nil {
		return err
	}
	if !confirm {
		fmt.Fprintln(out, "No changes were made.")
		return nil
	}
	return writeRepositoryManifest(m, out)
}

func detectedApplicationManifest(name, environment string, sql, cache, objectStorage, secrets, hasWorkload bool) application.Manifest {
	if environment == "" {
		environment = "dev"
	}
	return application.Manifest{
		Version:     application.CurrentVersion,
		Name:        name,
		Environment: environment,
		Services: application.Services{
			SQL:           sql,
			Cache:         cache,
			ObjectStorage: objectStorage,
			Secrets:       secrets,
		},
	}
}

func manifestFromDetectedProject(d appProjectDetection, quick bool) (application.Manifest, error) {
	if quick && len(d.ComposeCandidates) > 1 {
		return application.Manifest{}, usageError("multiple Compose files were detected", "Run 'baha app init' interactively to choose the application workload Compose file.")
	}
	if quick && len(d.AmbiguousServices) > 0 {
		return application.Manifest{}, usageError("ambiguous Compose service classification was detected", "Run 'baha app init' interactively to classify: "+strings.Join(d.AmbiguousServices, ", "))
	}
	sql, cache, objectStorage := d.SQL, d.Cache, d.ObjectStorage
	// Secret names discovered from env/example files are heuristic evidence only.
	// Quick mode must never promote them into required portable contract entries
	// without an explicit developer confirmation.
	secrets := false
	hasWorkload := d.Compose != "" && len(d.WorkloadServices) > 0
	if quick && !sql && !cache && !objectStorage && !secrets && !hasWorkload && !d.Metrics && !d.OTLP {
		return application.Manifest{}, usageError(
			"no unambiguous application requirements were detected",
			"Run 'baha app init' interactively or use explicit capability flags.",
		)
	}
	m := detectedApplicationManifest(d.Name, "dev", sql, cache, objectStorage, secrets, hasWorkload)
	if sqlNamed := quickNamedInstances(d.SQLInstances); len(sqlNamed) > 0 {
		m = application.WithSQLInstances(m, sqlNamed...)
	}
	if cacheNamed := quickNamedInstances(d.CacheInstances); len(cacheNamed) > 0 {
		m = application.WithCacheInstances(m, cacheNamed...)
	}
	if !quick {
		m = application.WithRequiredSecrets(m, d.SecretCandidates...)
	}
	if d.Compose != "" && len(d.WorkloadServices) > 0 {
		m = application.WithWorkload(m, d.Compose, d.WorkloadServices...)
	}
	if quick && d.Metrics {
		service, port, ok := detectedMetricsTarget(d, d.WorkloadServices)
		if !ok {
			return application.Manifest{}, usageError("metrics endpoint was detected but its workload service/port is ambiguous", "Run 'baha app init' interactively to confirm the metrics target.")
		}
		m = application.WithMetricsSource(m, "application", service, port, "/metrics")
	}
	if quick && d.OTLP {
		if len(d.OTLPSignals) == 0 {
			return application.Manifest{}, usageError("OTLP export was detected but the signal set is ambiguous", "Run 'baha app init' interactively to confirm traces, metrics and/or logs.")
		}
		m = application.WithOTLPTelemetry(m, d.OTLPSignals...)
	}
	if quick && len(d.RuntimePermissions) > 0 {
		if len(d.WorkloadServices) != 1 {
			return application.Manifest{}, usageError("runtime capability use was detected but the workload service scope is ambiguous", "Run 'baha app init' interactively to confirm the authorized workload service.")
		}
		for capabilityID, operations := range d.RuntimePermissions {
			m = application.WithRuntimePermission(m, capabilityID, d.WorkloadServices, operations...)
		}
	}
	if err := m.Validate(); err != nil {
		return application.Manifest{}, err
	}
	return m, nil
}

func applyGuidedSecretPolicies(m application.Manifest, policies []guidedSecretPolicy) application.Manifest {
	for _, policy := range policies {
		switch {
		case policy.Required && policy.Provision == "generate":
			m = application.WithGeneratedSecret(m, policy.Name, "random", 32)
		case !policy.Required && policy.Provision == "generate":
			m = application.WithOptionalGeneratedSecret(m, policy.Name, "random", 32)
		case policy.Required:
			m = application.WithRequiredSecrets(m, policy.Name)
		default:
			m = application.WithOptionalSecrets(m, policy.Name)
		}
	}
	return m
}

func writeRepositoryManifest(m application.Manifest, out io.Writer) error {
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
	if len(application.RequiredSecretNames(m)) > 0 {
		fmt.Fprintf(out, "required application secrets: %s\n", strings.Join(application.RequiredSecretNames(m), ", "))
	}
	fmt.Fprintln(out, "next: run 'baha up'")
	return nil
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
