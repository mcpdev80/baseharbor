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
	"strconv"
	"strings"
	"unicode"

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
	Postgres               bool
	PostgresSource         string
	PostgresInstances      []string
	Redis                  bool
	RedisSource            string
	RedisInstances         []string
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
		Usage:   "baha app init [--quick] | baha app init [NAME] [--environment ENV] [--postgres] [--postgres-instance NAME]... [--redis] [--redis-instance NAME]... [--s3] [--s3-bucket NAME]... [--secrets] [--require-secret NAME]...",
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

func detectAppProject(root string) (appProjectDetection, error) {
	result, err := repositoryinspect.Inspect(context.Background(), root)
	if err != nil {
		return appProjectDetection{}, err
	}
	d := appProjectDetection{
		Name:                   result.Application,
		ComposeCandidates:      append([]string(nil), result.ComposeCandidates...),
		Compose:                result.SelectedCompose,
		WorkloadServices:       append([]string(nil), result.WorkloadServices...),
		InfrastructureServices: append([]string(nil), result.InfrastructureServices...),
		AmbiguousServices:      append([]string(nil), result.AmbiguousServices...),
		Ports:                  append([]repositoryinspect.PortEvidence(nil), result.Ports...),
		SecretCandidates:       append([]string(nil), result.SecretCandidates...),
		SecretSources:          map[string]string{},
		RuntimePermissions:     map[string][]string{},
	}
	for name, source := range result.SecretSources {
		d.SecretSources[name] = source
	}
	for _, artifact := range result.Artifacts {
		if artifact.Kind == "env" {
			d.EnvFiles = append(d.EnvFiles, artifact.Path)
		}
	}
	for _, finding := range result.Findings {
		source := ""
		if len(finding.Evidence) > 0 {
			source = finding.Evidence[0].Path + " " + finding.Evidence[0].Detail
		}
		detected := finding.Confidence == repositoryinspect.ConfidenceDetected
		suggested := finding.Confidence == repositoryinspect.ConfidenceSuggested
		switch finding.Capability {
		case "database.sql":
			if !detected {
				continue
			}
			d.Postgres = true
			if finding.Name != "" {
				d.PostgresInstances = append(d.PostgresInstances, finding.Name)
			}
			if d.PostgresSource == "" {
				d.PostgresSource = source
			}
		case "cache.key-value":
			if !detected {
				continue
			}
			d.Redis = true
			if finding.Name != "" {
				d.RedisInstances = append(d.RedisInstances, finding.Name)
			}
			if d.RedisSource == "" {
				d.RedisSource = source
			}
		case "object-storage.s3":
			staticObjectStorageEvidence := false
			for _, evidence := range finding.Evidence {
				if evidence.Kind == repositoryinspect.EvidenceCompose || evidence.Kind == repositoryinspect.EvidenceEnv {
					staticObjectStorageEvidence = true
					break
				}
			}
			d.ObjectStorage = d.ObjectStorage || (detected && staticObjectStorageEvidence)
			d.ObjectStorageSuggested = d.ObjectStorageSuggested || suggested
			if d.ObjectStorageSource == "" {
				d.ObjectStorageSource = source
			}
			if len(finding.Operations) > 0 {
				for _, operation := range finding.Operations {
					d.RuntimePermissions["object-storage.s3/v1"] = append(
						d.RuntimePermissions["object-storage.s3/v1"],
						string(operation),
					)
				}
			}
		case "metrics":
			d.Metrics = d.Metrics || detected
			d.MetricsSuggested = d.MetricsSuggested || suggested
			if d.MetricsSource == "" {
				d.MetricsSource = source
			}
		case "telemetry.otlp":
			d.OTLP = d.OTLP || detected
			d.OTLPSuggested = d.OTLPSuggested || suggested
			if finding.Name != "" && (detected || suggested) {
				d.OTLPSignals = append(d.OTLPSignals, finding.Name)
			}
			if d.OTLPSource == "" {
				d.OTLPSource = source
			}
		case "logs":
			d.LogsSuggested = d.LogsSuggested || detected || suggested
		case "runtime-api":
			d.RuntimeAPI = d.RuntimeAPI || detected
		}
	}
	d.EnvFiles = uniqueSorted(d.EnvFiles)
	d.PostgresInstances = uniqueSorted(d.PostgresInstances)
	d.RedisInstances = uniqueSorted(d.RedisInstances)
	d.OTLPSignals = uniqueSorted(d.OTLPSignals)
	for capabilityID, operations := range d.RuntimePermissions {
		d.RuntimePermissions[capabilityID] = uniqueSorted(operations)
	}
	return d, nil
}

func detectComposeServices(path string) ([]composeServiceDetection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	inServices := false
	current := ""
	items := map[string]*composeServiceDetection{}
	for scanner.Scan() {
		raw := strings.TrimRight(scanner.Text(), " \t\r")
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent == 0 {
			inServices = trim == "services:"
			current = ""
			continue
		}
		if !inServices {
			continue
		}
		if indent == 2 && strings.HasSuffix(trim, ":") {
			name := strings.TrimSpace(strings.TrimSuffix(trim, ":"))
			if name == "" || strings.Contains(name, " ") {
				continue
			}
			current = name
			items[name] = &composeServiceDetection{Name: name}
			continue
		}
		if current == "" || indent < 4 {
			continue
		}
		item := items[current]
		lower := strings.ToLower(trim)
		if strings.HasPrefix(lower, "image:") {
			item.HasImage = true
		}
		if strings.HasPrefix(lower, "build:") {
			item.HasBuild = true
		}
		if lower == "ports:" || strings.HasPrefix(lower, "ports:") {
			item.HasPorts = true
		}
		combined := strings.ToLower(current + " " + trim)
		if strings.Contains(combined, "postgres") || strings.Contains(combined, "postgresql") {
			item.Postgres = true
		}
		if strings.Contains(combined, "redis") || strings.Contains(combined, "valkey") {
			item.Redis = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	result := make([]composeServiceDetection, 0, len(items))
	for _, item := range items {
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func readEnvNames(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var names []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		trim := strings.TrimSpace(scanner.Text())
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		trim = strings.TrimPrefix(trim, "export ")
		name, _, ok := strings.Cut(trim, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if validEnvName(name) {
			names = append(names, name)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return uniqueSorted(names), nil
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if !(r == '_' || unicode.IsLetter(r) || (i > 0 && unicode.IsDigit(r))) {
			return false
		}
	}
	return true
}

func likelySecretName(name string) bool {
	upper := strings.ToUpper(name)
	if strings.Contains(upper, "PUBLIC") || strings.HasSuffix(upper, "_URL") || strings.HasSuffix(upper, "_HOST") || strings.HasSuffix(upper, "_PORT") {
		return false
	}
	return upper == "SECRET_KEY" || strings.Contains(upper, "PASSWORD") || strings.HasSuffix(upper, "_SECRET") || strings.HasSuffix(upper, "_TOKEN") || strings.HasSuffix(upper, "_API_KEY") || strings.HasSuffix(upper, "_PRIVATE_KEY")
}

func slugifyAppName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > 63 {
		result = strings.Trim(result[:63], "-")
	}
	return result
}

func detectedLogicalInstanceName(serviceName, kind string) string {
	name := slugifyAppName(serviceName)
	prefixes := []string{kind + "-"}
	suffixes := []string{"-" + kind}
	if kind == "postgres" {
		prefixes = append(prefixes, "postgresql-", "pg-")
		suffixes = append(suffixes, "-postgresql", "-pg")
	} else {
		prefixes = append(prefixes, "valkey-", "redis-")
		suffixes = append(suffixes, "-valkey", "-redis")
	}
	for _, prefix := range prefixes {
		name = strings.TrimPrefix(name, prefix)
	}
	for _, suffix := range suffixes {
		name = strings.TrimSuffix(name, suffix)
	}
	if name == "" {
		return slugifyAppName(serviceName)
	}
	return name
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
		d.Postgres,
		d.Redis,
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

	var postgresInstances, redisInstances, objectStorageBuckets []string
	if selected[0] {
		postgresInstances, err = promptServiceInstances(reader, out, "PostgreSQL", d.PostgresInstances)
		if err != nil {
			return err
		}
	}
	if selected[1] {
		redisInstances, err = promptServiceInstances(reader, out, "Valkey / Redis", d.RedisInstances)
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
	if len(postgresInstances) > 0 {
		m = application.WithPostgresInstances(m, postgresInstances...)
	}
	if len(redisInstances) > 0 {
		m = application.WithRedisInstances(m, redisInstances...)
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

func detectedApplicationManifest(name, environment string, postgres, redis, objectStorage, secrets, hasWorkload bool) application.Manifest {
	if environment == "" {
		environment = "dev"
	}
	return application.Manifest{
		Version:     application.CurrentVersion,
		Name:        name,
		Environment: environment,
		Services: application.Services{
			Postgres:      postgres,
			Redis:         redis,
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
	postgres, redis, objectStorage := d.Postgres, d.Redis, d.ObjectStorage
	// Secret names discovered from env/example files are heuristic evidence only.
	// Quick mode must never promote them into required portable contract entries
	// without an explicit developer confirmation.
	secrets := false
	hasWorkload := d.Compose != "" && len(d.WorkloadServices) > 0
	if quick && !postgres && !redis && !objectStorage && !secrets && !hasWorkload && !d.Metrics && !d.OTLP {
		return application.Manifest{}, usageError(
			"no unambiguous application requirements were detected",
			"Run 'baha app init' interactively or use explicit capability flags.",
		)
	}
	m := detectedApplicationManifest(d.Name, "dev", postgres, redis, objectStorage, secrets, hasWorkload)
	if postgresNamed := quickNamedInstances(d.PostgresInstances); len(postgresNamed) > 0 {
		m = application.WithPostgresInstances(m, postgresNamed...)
	}
	if redisNamed := quickNamedInstances(d.RedisInstances); len(redisNamed) > 0 {
		m = application.WithRedisInstances(m, redisNamed...)
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

func printProjectDetection(out io.Writer, d appProjectDetection) {
	fmt.Fprintf(out, "✓ Application name: %s\n", d.Name)
	if d.Compose != "" {
		fmt.Fprintf(out, "✓ Compose file: %s (read-only)\n", d.Compose)
	} else if len(d.ComposeCandidates) > 1 {
		fmt.Fprintf(out, "? Compose file: %d candidates need confirmation\n", len(d.ComposeCandidates))
	} else {
		fmt.Fprintln(out, "- Compose file: not detected")
	}
	if len(d.WorkloadServices) > 0 {
		fmt.Fprintf(out, "✓ Application workload: %s\n", strings.Join(d.WorkloadServices, ", "))
	}
	if len(d.InfrastructureServices) > 0 {
		fmt.Fprintf(out, "✓ Replaceable repository infrastructure: %s\n", strings.Join(d.InfrastructureServices, ", "))
	}
	if len(d.AmbiguousServices) > 0 {
		fmt.Fprintf(out, "? Compose services need classification: %s\n", strings.Join(d.AmbiguousServices, ", "))
	}
	if d.Postgres {
		fmt.Fprintf(out, "✓ SQL Database detected (PostgreSQL-compatible evidence: %s)\n", d.PostgresSource)
		if len(d.PostgresInstances) > 1 {
			fmt.Fprintf(out, "  logical instances proposed: %s\n", strings.Join(d.PostgresInstances, ", "))
		}
	} else {
		fmt.Fprintln(out, "- SQL Database not detected")
	}
	if d.Redis {
		fmt.Fprintf(out, "✓ Cache detected (Redis/Valkey-compatible evidence: %s)\n", d.RedisSource)
		if len(d.RedisInstances) > 1 {
			fmt.Fprintf(out, "  logical instances proposed: %s\n", strings.Join(d.RedisInstances, ", "))
		}
	} else {
		fmt.Fprintln(out, "- Cache not detected")
	}
	switch {
	case d.ObjectStorage:
		fmt.Fprintf(out, "✓ Object Storage detected (S3-compatible evidence: %s)\n", d.ObjectStorageSource)
	case d.ObjectStorageSuggested:
		fmt.Fprintf(out, "? Object Storage suggested (S3-compatible evidence: %s)\n", d.ObjectStorageSource)
	default:
		fmt.Fprintln(out, "- Object Storage not detected")
	}
	switch {
	case d.Metrics:
		fmt.Fprintf(out, "✓ Metrics detected at /metrics (%s)\n", d.MetricsSource)
	case d.MetricsSuggested:
		fmt.Fprintf(out, "? Metrics suggested (%s)\n", d.MetricsSource)
	}
	switch {
	case d.OTLP && len(d.OTLPSignals) > 0:
		fmt.Fprintf(out, "✓ Observability detected (OTLP: %s)\n", strings.Join(d.OTLPSignals, ", "))
	case d.OTLP:
		fmt.Fprintln(out, "? Observability detected via OTLP; signal set needs confirmation")
	case d.OTLPSuggested:
		fmt.Fprintln(out, "? Observability via OTLP suggested")
	}
	if d.LogsSuggested {
		fmt.Fprintln(out, "? Application log collection available for the selected workload")
	}
	if d.RuntimeAPI {
		fmt.Fprintln(out, "✓ BaseHarbor Runtime API usage detected")
	}
	for capabilityID, operations := range d.RuntimePermissions {
		fmt.Fprintf(out, "✓ Runtime operations for %s: %s\n", capabilityID, strings.Join(operations, ", "))
	}
	if len(d.SecretCandidates) > 0 {
		fmt.Fprintln(out, "✓ Potential required secret names:")
		for _, name := range d.SecretCandidates {
			fmt.Fprintf(out, "    %s (%s)\n", name, d.SecretSources[name])
		}
	} else {
		fmt.Fprintln(out, "- Required application secrets not detected")
	}
}

func promptCapabilityList(reader *bufio.Reader, out io.Writer, defaults []bool, allowNone bool) ([]bool, error) {
	labels := []string{
		"SQL Database (PostgreSQL-compatible evidence)",
		"Cache (Redis/Valkey-compatible evidence)",
		"Object Storage (S3-compatible)",
		"Managed Secrets",
		"Metrics (/metrics)",
		"OTLP telemetry",
		"Application logs",
	}
	fmt.Fprintln(out, "\nSelect application capabilities (Enter keeps detected defaults; suggested capabilities remain opt-in):")
	for i, label := range labels {
		mark := " "
		if defaults[i] {
			mark = "x"
		}
		fmt.Fprintf(out, "[%s] %d. %s\n", mark, i+1, label)
	}
	line, err := readPrompt(reader, out, "> ")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(line) == "" {
		selected := append([]bool(nil), defaults...)
		if !anySelected(selected) && !allowNone {
			return nil, errors.New("select at least one backend capability or configure an application workload")
		}
		return selected, nil
	}
	selected := make([]bool, len(labels))
	for _, raw := range strings.Split(line, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 1 || n > len(labels) {
			return nil, fmt.Errorf("invalid service selection %q", raw)
		}
		selected[n-1] = true
	}
	if !anySelected(selected) && !allowNone {
		return nil, errors.New("select at least one backend capability or configure an application workload")
	}
	return selected, nil
}

func anySelected(values []bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
}

func detectedMetricsTarget(d appProjectDetection, workloadServices []string) (string, int, bool) {
	allowed := map[string]struct{}{}
	for _, service := range workloadServices {
		allowed[service] = struct{}{}
	}
	type target struct {
		service string
		port    int
	}
	var targets []target
	for _, item := range d.Ports {
		if _, ok := allowed[item.Service]; !ok {
			continue
		}
		port, ok := composeTargetPort(item.Value)
		if !ok {
			continue
		}
		targets = append(targets, target{service: item.Service, port: port})
	}
	if len(targets) != 1 {
		return "", 0, false
	}
	return targets[0].service, targets[0].port, true
}

func composeTargetPort(value string) (int, bool) {
	value = strings.TrimSpace(strings.Trim(value, "\"'"))
	parts := strings.Split(value, ":")
	target := parts[len(parts)-1]
	target = strings.TrimSuffix(target, "/tcp")
	target = strings.TrimSuffix(target, "/udp")
	if strings.Contains(target, "-") {
		return 0, false
	}
	port, err := strconv.Atoi(target)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}

func promptMetricsTarget(reader *bufio.Reader, out io.Writer, workloadServices []string) (string, int, error) {
	if len(workloadServices) == 0 {
		return "", 0, errors.New("metrics collection requires an application workload service")
	}
	defaultService := workloadServices[0]
	service, err := promptLine(reader, out, "Metrics workload service", defaultService)
	if err != nil {
		return "", 0, err
	}
	found := false
	for _, candidate := range workloadServices {
		if candidate == service {
			found = true
			break
		}
	}
	if !found {
		return "", 0, fmt.Errorf("metrics service %q is not a selected workload service", service)
	}
	value, err := promptLine(reader, out, "Metrics container port", "")
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid metrics container port %q", value)
	}
	return service, port, nil
}

func runtimePermissionServices(reader *bufio.Reader, out io.Writer, workloadServices []string) ([]string, error) {
	if len(workloadServices) == 0 {
		return nil, errors.New("runtime capability permissions require an application workload service")
	}
	if len(workloadServices) == 1 {
		return append([]string(nil), workloadServices...), nil
	}
	value, err := promptLine(reader, out, "Workload services allowed to use detected Runtime API operations (comma-separated)", strings.Join(workloadServices, ","))
	if err != nil {
		return nil, err
	}
	selected := uniqueSorted(strings.Split(value, ","))
	known := map[string]struct{}{}
	for _, service := range workloadServices {
		known[service] = struct{}{}
	}
	for _, service := range selected {
		if _, ok := known[service]; !ok {
			return nil, fmt.Errorf("runtime permission service %q is not a selected workload service", service)
		}
	}
	return selected, nil
}

func promptAmbiguousComposeServices(reader *bufio.Reader, out io.Writer, services []string) ([]string, error) {
	services = uniqueSorted(services)
	if len(services) == 0 {
		return nil, nil
	}
	fmt.Fprintln(out, "\nThese Compose services look infrastructure-like but could not be classified safely.")
	fmt.Fprintln(out, "Select any that are application workload services (Enter = none; remaining services stay replaceable infrastructure):")
	for i, service := range services {
		fmt.Fprintf(out, "  %d. %s\n", i+1, service)
	}
	line, err := readPrompt(reader, out, "> ")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(line) == "" {
		return nil, nil
	}
	var selected []string
	for _, raw := range strings.Split(line, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 1 || n > len(services) {
			return nil, fmt.Errorf("invalid ambiguous service selection %q", raw)
		}
		selected = append(selected, services[n-1])
	}
	return uniqueSorted(selected), nil
}

func promptServiceInstances(reader *bufio.Reader, out io.Writer, label string, detected []string) ([]string, error) {
	detected = uniqueSorted(detected)
	defaultValue := "default"
	if len(detected) > 1 {
		defaultValue = strings.Join(detected, ",")
	}
	line, err := promptLine(reader, out, label+" instances (comma-separated)", defaultValue)
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" || line == "default" {
		return nil, nil
	}
	var names []string
	for _, raw := range strings.Split(line, ",") {
		name := slugifyAppName(raw)
		if name == "" {
			return nil, fmt.Errorf("invalid %s instance name %q", label, raw)
		}
		names = append(names, name)
	}
	return uniqueSorted(names), nil
}

func quickNamedInstances(detected []string) []string {
	detected = uniqueSorted(detected)
	if len(detected) > 1 {
		return detected
	}
	return nil
}

func promptSecretPolicies(reader *bufio.Reader, out io.Writer, candidates []string, sources map[string]string) ([]guidedSecretPolicy, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	fmt.Fprintln(out, "\nApplication secrets")
	fmt.Fprintln(out, "Detected names are evidence only. Repository values are never displayed or imported.")
	var policies []guidedSecretPolicy
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		source := sources[candidate]
		fmt.Fprintf(out, "\nPotential secret detected\n  Source: %s\n  Detected variable: %s\n", source, candidate)
		use, err := promptYesNo(reader, out, "Manage this application secret with BaseHarbor?", true)
		if err != nil {
			return nil, err
		}
		if !use {
			continue
		}
		name, err := promptLine(reader, out, "BaseHarbor secret name", candidate)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, errors.New("application secret name cannot be empty")
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate application secret %q", name)
		}
		seen[name] = struct{}{}
		required, err := promptYesNo(reader, out, "Required for application startup?", true)
		if err != nil {
			return nil, err
		}
		provision, err := promptSecretProvisionPolicy(reader, out)
		if err != nil {
			return nil, err
		}
		policies = append(policies, guidedSecretPolicy{
			Name: name, Source: source, Required: required, Provision: provision,
		})
	}
	return policies, nil
}

func promptSecretProvisionPolicy(reader *bufio.Reader, out io.Writer) (string, error) {
	fmt.Fprintln(out, "How should it be provided?")
	fmt.Fprintln(out, "  1. Ask for value during first apply")
	fmt.Fprintln(out, "  2. Generate automatically")
	fmt.Fprintln(out, "  3. Configure later")
	value, err := promptLine(reader, out, "Selection", "1")
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(value) {
	case "1":
		return "prompt", nil
	case "2":
		return "generate", nil
	case "3":
		return "later", nil
	default:
		return "", fmt.Errorf("invalid secret provision selection %q", value)
	}
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

func printManagedCredentialSummary(out io.Writer, selected []bool, runtimePermissions bool) {
	var managed []string
	if len(selected) > 0 && selected[0] {
		managed = append(managed, "SQL service credentials")
	}
	if len(selected) > 1 && selected[1] {
		managed = append(managed, "Cache service credentials")
	}
	if len(selected) > 2 && selected[2] {
		managed = append(managed, "Object-storage access credentials")
	}
	if runtimePermissions {
		managed = append(managed, "Runtime identity / mTLS credentials")
	}
	if len(managed) == 0 {
		return
	}
	fmt.Fprintln(out, "\nManaged automatically by BaseHarbor")
	for _, item := range managed {
		fmt.Fprintf(out, "  - %s\n", item)
	}
	fmt.Fprintln(out, "You do not need to create or enter these managed credentials.")
}

func printAdoptionSummary(out io.Writer, m application.Manifest, detected appProjectDetection, policies []guidedSecretPolicy) {
	fmt.Fprintln(out, "\nAdoption summary")
	fmt.Fprintln(out, "\nApplication")
	fmt.Fprintf(out, "  Name          %s\n", m.Name)
	fmt.Fprintf(out, "  Environment   %s\n", m.Environment)

	if m.Workload.Compose != "" || len(m.Workload.Services) > 0 {
		fmt.Fprintln(out, "\nWorkload")
		if m.Workload.Compose != "" {
			fmt.Fprintf(out, "  Compose       %s (repository-owned, read-only)\n", m.Workload.Compose)
		}
		if len(m.Workload.Services) > 0 {
			fmt.Fprintf(out, "  Services      %s\n", strings.Join(m.Workload.Services, ", "))
		}
	}

	if m.Services.Postgres || m.Services.Redis || m.Services.ObjectStorage {
		fmt.Fprintln(out, "\nManaged services")
		if m.Services.Postgres {
			fmt.Fprintf(out, "  SQL Database  %s; default provider PostgreSQL\n", adoptionOrigin(detected.Postgres))
		}
		if m.Services.Redis {
			fmt.Fprintf(out, "  Cache         %s; default provider Valkey/Redis-compatible\n", adoptionOrigin(detected.Redis))
		}
		if m.Services.ObjectStorage {
			fmt.Fprintf(out, "  Object Storage %s; S3-compatible\n", adoptionOrigin(detected.ObjectStorage))
		}
	}

	if application.HasMetricsSources(m) || application.HasOTLPTelemetry(m) || application.HasLogsCollection(m) {
		fmt.Fprintln(out, "\nObservability")
		if application.HasMetricsSources(m) {
			fmt.Fprintln(out, "  Metrics       /metrics")
		}
		if application.HasOTLPTelemetry(m) {
			fmt.Fprintf(out, "  OTLP          %s\n", strings.Join(m.Telemetry.OTLP.Signals, ", "))
		}
		if application.HasLogsCollection(m) {
			fmt.Fprintln(out, "  Logs          application")
		}
	}

	printGuidedSecretSummary(out, policies)

	if len(m.Runtime.Permissions) > 0 {
		fmt.Fprintln(out, "\nRuntime permissions")
		for _, permission := range m.Runtime.Permissions {
			fmt.Fprintf(out, "  %s\n", permission.Capability)
			fmt.Fprintf(out, "    services: %s\n", strings.Join(permission.Services, ", "))
			fmt.Fprintf(out, "    operations: %s\n", strings.Join(permission.Operations, ", "))
		}
	}
}

func adoptionOrigin(detected bool) string {
	if detected {
		return "detected and confirmed"
	}
	return "user confirmed"
}

func printGuidedSecretSummary(out io.Writer, policies []guidedSecretPolicy) {
	if len(policies) == 0 {
		return
	}
	fmt.Fprintln(out, "\nSecret policy")
	for _, policy := range policies {
		requirement := "optional"
		if policy.Required {
			requirement = "required for startup"
		}
		action := "configure later"
		switch policy.Provision {
		case "prompt":
			action = "ask securely during first apply"
		case "generate":
			action = "generate automatically"
		}
		fmt.Fprintf(out, "  %s: %s; %s\n", policy.Name, requirement, action)
	}
}

func promptCompose(reader *bufio.Reader, out io.Writer, candidates []string) (string, error) {
	fmt.Fprintln(out, "\nMultiple Compose files were detected. Select the application workload:")
	for i, candidate := range candidates {
		fmt.Fprintf(out, "  %d. %s\n", i+1, candidate)
	}
	line, err := readPrompt(reader, out, "> ")
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(candidates) {
		return "", fmt.Errorf("invalid Compose selection %q", line)
	}
	return candidates[n-1], nil
}

func promptLine(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	prompt := label
	if defaultValue != "" {
		prompt += " [" + defaultValue + "]"
	}
	line, err := readPrompt(reader, out, prompt+": ")
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}

func promptYesNo(reader *bufio.Reader, out io.Writer, label string, defaultYes bool) (bool, error) {
	suffix := "[Y/n]"
	if !defaultYes {
		suffix = "[y/N]"
	}
	line, err := readPrompt(reader, out, label+" "+suffix+" ")
	if err != nil {
		return false, err
	}
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return defaultYes, nil
	}
	switch line {
	case "y", "yes", "j", "ja":
		return true, nil
	case "n", "no", "nein":
		return false, nil
	default:
		return false, fmt.Errorf("expected yes or no, got %q", line)
	}
}

func readPrompt(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if errors.Is(err, io.EOF) && line == "" {
		return "", io.EOF
	}
	return strings.TrimRight(line, "\r\n"), nil
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
		fmt.Fprintln(out, "next: run 'baha app preflight' to check secret readiness, then 'baha app apply'")
	} else {
		fmt.Fprintln(out, "next: run 'baha app apply'")
	}
	return nil
}

func appInitReaderIsTerminal(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok {
		return true
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
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
