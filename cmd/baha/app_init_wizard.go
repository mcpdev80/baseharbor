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
	OTLPSource             string
	LogsSuggested          bool
	RuntimeAPI             bool
	RuntimePermissions     map[string][]string
	Ports                  []repositoryinspect.PortEvidence
	SecretCandidates       []string
	SecretSources          map[string]string
	EnvFiles               []string
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
			return runAppInitWizard(detected, out)
		},
	}
}

func detectAppProject(root string) (appProjectDetection, error) {
	result, err := repositoryinspect.Inspect(context.Background(), root)
	if err != nil {
		return appProjectDetection{}, err
	}
	d := appProjectDetection{
		Name:              result.Application,
		ComposeCandidates: append([]string(nil), result.ComposeCandidates...),
		Compose:           result.SelectedCompose,
		WorkloadServices:       append([]string(nil), result.WorkloadServices...),
		InfrastructureServices: append([]string(nil), result.InfrastructureServices...),
		Ports:                  append([]repositoryinspect.PortEvidence(nil), result.Ports...),
		SecretCandidates:       append([]string(nil), result.SecretCandidates...),
		SecretSources:          map[string]string{},
		RuntimePermissions:    map[string][]string{},
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
		if finding.Confidence != repositoryinspect.ConfidenceDetected {
			continue
		}
		source := ""
		if len(finding.Evidence) > 0 {
			source = finding.Evidence[0].Path + " " + finding.Evidence[0].Detail
		}
		switch finding.Capability {
		case "database.sql":
			d.Postgres = true
			if finding.Name != "" {
				d.PostgresInstances = append(d.PostgresInstances, finding.Name)
			}
			if d.PostgresSource == "" {
				d.PostgresSource = source
			}
		case "cache.key-value":
			d.Redis = true
			if finding.Name != "" {
				d.RedisInstances = append(d.RedisInstances, finding.Name)
			}
			if d.RedisSource == "" {
				d.RedisSource = source
			}
		}
	}
	d.EnvFiles = uniqueSorted(d.EnvFiles)
	d.PostgresInstances = uniqueSorted(d.PostgresInstances)
	d.RedisInstances = uniqueSorted(d.RedisInstances)
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

func runAppInitWizard(d appProjectDetection, out io.Writer) error {
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
	}

	defaults := []bool{d.Postgres, d.Redis, false, len(d.SecretCandidates) > 0}
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

	required := []string(nil)
	if selected[3] {
		required, err = promptSecretCandidates(reader, out, d.SecretCandidates, d.SecretSources)
		if err != nil {
			return err
		}
		additional, err := promptLine(reader, out, "Additional required secret names (comma-separated, Enter for none)", "")
		if err != nil {
			return err
		}
		for _, item := range strings.Split(additional, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				required = append(required, item)
			}
		}
		required = uniqueSorted(required)
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
	m = application.WithRequiredSecrets(m, required...)
	if compose != "" && len(workloadServices) > 0 {
		m = application.WithWorkload(m, filepath.ToSlash(compose), workloadServices...)
	}
	if err := m.Validate(); err != nil {
		return err
	}

	fmt.Fprintln(out, "\nManifest preview:")
	fmt.Fprintln(out, "----------------------------------------")
	fmt.Fprint(out, m.YAML())
	fmt.Fprintln(out, "----------------------------------------")
	confirm, err := promptYesNo(reader, out, "Write ./baseharbor.yaml?", true)
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
	postgres, redis := d.Postgres, d.Redis
	// Secret names discovered from env/example files are heuristic evidence only.
	// Quick mode must never promote them into required portable contract entries
	// without an explicit developer confirmation.
	secrets := false
	hasWorkload := d.Compose != "" && len(d.WorkloadServices) > 0
	if quick && !postgres && !redis && !secrets && !hasWorkload {
		return application.Manifest{}, usageError(
			"no unambiguous application requirements were detected",
			"Run 'baha app init' interactively or use explicit capability flags.",
		)
	}
	m := detectedApplicationManifest(d.Name, "dev", postgres, redis, false, secrets, hasWorkload)
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
	if err := m.Validate(); err != nil {
		return application.Manifest{}, err
	}
	return m, nil
}

func printProjectDetection(out io.Writer, d appProjectDetection) {
	fmt.Fprintf(out, "✓ Application name: %s\n", d.Name)
	if d.Compose != "" {
		fmt.Fprintf(out, "✓ Compose file: %s\n", d.Compose)
	} else if len(d.ComposeCandidates) > 1 {
		fmt.Fprintf(out, "? Compose file: %d candidates need confirmation\n", len(d.ComposeCandidates))
	} else {
		fmt.Fprintln(out, "- Compose file: not detected")
	}
	if len(d.WorkloadServices) > 0 {
		fmt.Fprintf(out, "✓ Workload services: %s\n", strings.Join(d.WorkloadServices, ", "))
	}
	if d.Postgres {
		fmt.Fprintf(out, "✓ PostgreSQL detected from %s\n", d.PostgresSource)
		if len(d.PostgresInstances) > 1 {
			fmt.Fprintf(out, "  logical instances proposed: %s\n", strings.Join(d.PostgresInstances, ", "))
		}
	} else {
		fmt.Fprintln(out, "- PostgreSQL not detected")
	}
	if d.Redis {
		fmt.Fprintf(out, "✓ Redis/Valkey detected from %s\n", d.RedisSource)
		if len(d.RedisInstances) > 1 {
			fmt.Fprintf(out, "  logical instances proposed: %s\n", strings.Join(d.RedisInstances, ", "))
		}
	} else {
		fmt.Fprintln(out, "- Redis/Valkey not detected")
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
	labels := []string{"PostgreSQL", "Valkey / Redis", "S3-compatible Object Storage", "Managed Secrets"}
	fmt.Fprintln(out, "\nSelect required services (Enter keeps detected/default selection; otherwise enter numbers such as 1,3):")
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
		if !selected[0] && !selected[1] && !selected[2] && !selected[3] && !allowNone {
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
	if !selected[0] && !selected[1] && !selected[2] && !selected[3] && !allowNone {
		return nil, errors.New("select at least one backend capability or configure an application workload")
	}
	return selected, nil
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

func promptSecretCandidates(reader *bufio.Reader, out io.Writer, candidates []string, sources map[string]string) ([]string, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	fmt.Fprintln(out, "\nDetected potential required application secrets (Enter keeps all; otherwise enter selected numbers):")
	for i, name := range candidates {
		fmt.Fprintf(out, "[x] %d. %s (%s)\n", i+1, name, sources[name])
	}
	line, err := readPrompt(reader, out, "> ")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(line) == "" {
		return append([]string(nil), candidates...), nil
	}
	var selected []string
	for _, raw := range strings.Split(line, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 1 || n > len(candidates) {
			return nil, fmt.Errorf("invalid secret selection %q", raw)
		}
		selected = append(selected, candidates[n-1])
	}
	return uniqueSorted(selected), nil
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
