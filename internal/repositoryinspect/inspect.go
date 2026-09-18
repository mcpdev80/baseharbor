package repositoryinspect

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const maxInspectionFileSize = 2 << 20

var ignoredDirectories = map[string]struct{}{
	".git": {}, ".baseharbor": {}, "node_modules": {}, "vendor": {},
	"dist": {}, "build": {}, ".venv": {}, "venv": {}, "__pycache__": {},
}

func DefaultEngine() Engine {
	return Engine{Detectors: []Detector{
		sqlDetector{},
		keyValueDetector{},
	}}
}

func Inspect(ctx context.Context, root string) (Result, error) {
	return DefaultEngine().Inspect(ctx, root)
}

func (e Engine) Inspect(ctx context.Context, root string) (Result, error) {
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, fmt.Errorf("resolve inspection root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Result{}, fmt.Errorf("inspect repository root: %w", err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("inspection root %s is not a directory", absRoot)
	}

	snapshot, artifacts, err := collectSnapshot(ctx, absRoot)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Root:          absRoot,
		Application:   slugify(filepath.Base(absRoot)),
		Artifacts:     artifacts,
		SecretSources: map[string]string{},
	}
	if result.Application == "" {
		result.Application = "app"
	}

	if _, ok := snapshot.Files["baseharbor.yaml"]; ok {
		result.ExistingManifest = "baseharbor.yaml"
	}

	result.ComposeCandidates = artifactPaths(artifacts, "compose")
	if len(result.ComposeCandidates) == 1 {
		result.SelectedCompose = result.ComposeCandidates[0]
		services := detectComposeServices(snapshot.Files[result.SelectedCompose])
		for _, service := range services {
			if !service.Postgres && !service.Redis && (service.HasBuild || service.HasImage || service.HasPorts) {
				result.WorkloadServices = append(result.WorkloadServices, service.Name)
			}
			for _, port := range service.Ports {
				result.Ports = append(result.Ports, PortEvidence{
					Path: result.SelectedCompose, Service: service.Name, Value: port,
				})
			}
			if service.HealthCheck {
				result.HealthChecks = append(result.HealthChecks, Evidence{
					Kind: EvidenceHealth, Path: result.SelectedCompose,
					Detail: "compose service " + service.Name + " declares healthcheck",
				})
			}
		}
	}

	for _, rel := range artifactPaths(artifacts, "env") {
		for _, name := range readEnvNames(snapshot.Files[rel]) {
			if likelySecretName(name) {
				result.SecretCandidates = append(result.SecretCandidates, name)
				if _, exists := result.SecretSources[name]; !exists {
					result.SecretSources[name] = rel
				}
			}
		}
	}
	result.SecretCandidates = uniqueSorted(result.SecretCandidates)
	result.WorkloadServices = uniqueSorted(result.WorkloadServices)

	for _, detector := range e.Detectors {
		if detector == nil {
			continue
		}
		findings, err := detector.Detect(ctx, snapshot)
		if err != nil {
			return Result{}, fmt.Errorf("repository detector %s: %w", detector.Name(), err)
		}
		result.Findings = mergeFindings(result.Findings, findings)
	}
	sortResult(&result)
	return result, nil
}

func collectSnapshot(ctx context.Context, root string) (Snapshot, []Artifact, error) {
	snapshot := Snapshot{Root: root, Files: map[string][]byte{}}
	var artifacts []Artifact
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			if _, ignored := ignoredDirectories[entry.Name()]; ignored {
				return filepath.SkipDir
			}
			return nil
		}
		rel = filepath.ToSlash(rel)
		kind, read := classifyFile(rel)
		if !read {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxInspectionFileSize {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot.Files[rel] = data
		if kind != "" {
			artifacts = append(artifacts, Artifact{Kind: kind, Path: rel})
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Snapshot{}, nil, err
		}
		return Snapshot{}, nil, fmt.Errorf("inspect repository: %w", err)
	}
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].Kind != artifacts[j].Kind {
			return artifacts[i].Kind < artifacts[j].Kind
		}
		return artifacts[i].Path < artifacts[j].Path
	})
	return snapshot, artifacts, nil
}

func classifyFile(rel string) (string, bool) {
	base := strings.ToLower(filepath.Base(rel))
	switch base {
	case "baseharbor.yaml":
		return "baseharbor-manifest", true
	case "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml":
		return "compose", true
	case "dockerfile":
		return "dockerfile", true
	case ".env.example", ".env.template", ".env.sample", ".env":
		return "env", true
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"go.mod", "go.sum", "pyproject.toml", "requirements.txt", "poetry.lock",
		"cargo.toml", "cargo.lock":
		return "dependency", true
	}
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx",
		".json", ".yaml", ".yml", ".toml", ".ini", ".conf", ".properties":
		return "", true
	default:
		return "", false
	}
}

type composeService struct {
	Name        string
	Postgres    bool
	Redis       bool
	HasBuild    bool
	HasImage    bool
	HasPorts    bool
	Ports       []string
	HealthCheck bool
}

func detectComposeServices(data []byte) []composeService {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	inServices := false
	current := ""
	items := map[string]*composeService{}
	inPorts := false
	for scanner.Scan() {
		raw := strings.TrimRight(scanner.Text(), " 	")
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent == 0 {
			inServices = trim == "services:"
			current = ""
			inPorts = false
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
			items[name] = &composeService{Name: name}
			inPorts = false
			continue
		}
		if current == "" || indent < 4 {
			continue
		}
		item := items[current]
		lower := strings.ToLower(trim)
		switch {
		case strings.HasPrefix(lower, "image:"):
			item.HasImage = true
		case strings.HasPrefix(lower, "build:"):
			item.HasBuild = true
		case lower == "ports:" || strings.HasPrefix(lower, "ports:"):
			item.HasPorts = true
			inPorts = true
		case lower == "healthcheck:" || strings.HasPrefix(lower, "healthcheck:"):
			item.HealthCheck = true
			inPorts = false
		default:
			if indent <= 4 {
				inPorts = false
			}
		}
		if inPorts && strings.HasPrefix(trim, "-") {
			value := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trim, "-")), ""'")
			if value != "" {
				item.Ports = append(item.Ports, value)
			}
		}
		combined := strings.ToLower(current + " " + trim)
		if strings.Contains(combined, "postgres") || strings.Contains(combined, "postgresql") {
			item.Postgres = true
		}
		if strings.Contains(combined, "redis") || strings.Contains(combined, "valkey") {
			item.Redis = true
		}
	}
	result := make([]composeService, 0, len(items))
	for _, item := range items {
		item.Ports = uniqueSorted(item.Ports)
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

type sqlDetector struct{}

func (sqlDetector) Name() string { return "database.sql" }

func (sqlDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	return detectCapability(ctx, snapshot, "database.sql", sqlSignals()), nil
}

type keyValueDetector struct{}

func (keyValueDetector) Name() string { return "cache.key-value" }

func (keyValueDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	return detectCapability(ctx, snapshot, "cache.key-value", keyValueSignals()), nil
}

type signalSet struct {
	env        []string
	compose    []string
	dependency []string
	imports    []string
	config     []string
}

func sqlSignals() signalSet {
	return signalSet{
		env: []string{"DATABASE_URL", "POSTGRES_URL", "POSTGRESQL_URL"},
		compose: []string{"postgres", "postgresql"},
		dependency: []string{
			"pg", "postgres", "postgresql", "pgx", "psycopg", "asyncpg", "sqlalchemy",
		},
		imports: []string{"pgx", "psycopg", "asyncpg", "sequelize", "typeorm", "prisma", "postgres"},
		config: []string{"jdbc:postgresql:", "postgresql://", "postgres://"},
	}
}

func keyValueSignals() signalSet {
	return signalSet{
		env: []string{"REDIS_URL", "VALKEY_URL"},
		compose: []string{"redis", "valkey"},
		dependency: []string{
			"ioredis", "redis", "go-redis", "valkey",
		},
		imports: []string{"ioredis", "redis", "go-redis", "valkey"},
		config: []string{"redis://", "rediss://"},
	}
}

func detectCapability(ctx context.Context, snapshot Snapshot, capability string, signals signalSet) []Finding {
	var detected, suggested, possible []Evidence
	for path, data := range snapshot.Files {
		if ctx.Err() != nil {
			break
		}
		lower := strings.ToLower(string(data))
		base := strings.ToLower(filepath.Base(path))
		if isComposeFile(base) {
			for _, service := range detectComposeServices(data) {
				combined := strings.ToLower(service.Name + " " + string(data))
				if containsAny(combined, signals.compose) {
					detected = append(detected, Evidence{
						Kind: EvidenceCompose, Path: path,
						Detail: "compose service " + service.Name + " matches " + capability,
					})
					break
				}
			}
		}
		if isEnvFile(base) {
			for _, name := range readEnvNames(data) {
				if containsExactFold(signals.env, name) {
					detected = append(detected, Evidence{
						Kind: EvidenceEnv, Path: path, Detail: "variable " + name,
					})
				}
			}
		}
		if isDependencyFile(base) && containsAnyToken(lower, signals.dependency) {
			suggested = append(suggested, Evidence{
				Kind: EvidenceDependency, Path: path,
				Detail: "dependency metadata references a compatible client/provider",
			})
		}
		if isSourceFile(base) && containsAny(lower, signals.imports) {
			suggested = append(suggested, Evidence{
				Kind: EvidenceImport, Path: path,
				Detail: "source imports/references a compatible client",
			})
		}
		if !isEnvFile(base) && containsAny(lower, signals.config) {
			possible = append(possible, Evidence{
				Kind: EvidenceConfig, Path: path,
				Detail: "configuration contains a compatible endpoint pattern",
			})
		}
	}
	switch {
	case len(detected) > 0:
		return []Finding{{Capability: capability, Confidence: ConfidenceDetected, Evidence: uniqueEvidence(detected)}}
	case len(suggested) > 0:
		return []Finding{{Capability: capability, Confidence: ConfidenceSuggested, Evidence: uniqueEvidence(suggested)}}
	case len(possible) > 0:
		return []Finding{{Capability: capability, Confidence: ConfidencePossible, Evidence: uniqueEvidence(possible)}}
	default:
		return nil
	}
}

func readEnvNames(data []byte) []string {
	var names []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
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
	return uniqueSorted(names)
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
	if strings.Contains(upper, "PUBLIC") || strings.HasSuffix(upper, "_URL") ||
		strings.HasSuffix(upper, "_HOST") || strings.HasSuffix(upper, "_PORT") {
		return false
	}
	return upper == "SECRET_KEY" || strings.Contains(upper, "PASSWORD") ||
		strings.HasSuffix(upper, "_SECRET") || strings.HasSuffix(upper, "_TOKEN") ||
		strings.HasSuffix(upper, "_API_KEY") || strings.HasSuffix(upper, "_PRIVATE_KEY")
}

func artifactPaths(artifacts []Artifact, kind string) []string {
	var paths []string
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			paths = append(paths, artifact.Path)
		}
	}
	return uniqueSorted(paths)
}

func isComposeFile(base string) bool {
	switch base {
	case "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml":
		return true
	default:
		return false
	}
}

func isEnvFile(base string) bool {
	switch base {
	case ".env", ".env.example", ".env.template", ".env.sample":
		return true
	default:
		return false
	}
}

func isDependencyFile(base string) bool {
	switch base {
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"go.mod", "go.sum", "pyproject.toml", "requirements.txt", "poetry.lock",
		"cargo.toml", "cargo.lock":
		return true
	default:
		return false
	}
}

func isSourceFile(base string) bool {
	switch strings.ToLower(filepath.Ext(base)) {
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx":
		return true
	default:
		return false
	}
}

func containsExactFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func containsAnyToken(value string, needles []string) bool {
	for _, needle := range needles {
		n := strings.ToLower(needle)
		if strings.Contains(value, """+n+""") ||
			strings.Contains(value, "'"+n+"'") ||
			strings.Contains(value, "/"+n) ||
			strings.Contains(value, n+"/") ||
			strings.Contains(value, n+"@") ||
			strings.Contains(value, n+" ") ||
			strings.Contains(value, " "+n) {
			return true
		}
	}
	return false
}

func mergeFindings(current []Finding, incoming []Finding) []Finding {
	index := map[string]int{}
	for i, finding := range current {
		index[finding.Capability+" "+finding.Name] = i
	}
	for _, finding := range incoming {
		key := finding.Capability + " " + finding.Name
		if i, exists := index[key]; exists {
			if confidenceRank(finding.Confidence) > confidenceRank(current[i].Confidence) {
				current[i].Confidence = finding.Confidence
			}
			current[i].Evidence = uniqueEvidence(append(current[i].Evidence, finding.Evidence...))
			continue
		}
		finding.Evidence = uniqueEvidence(finding.Evidence)
		current = append(current, finding)
		index[key] = len(current) - 1
	}
	return current
}

func confidenceRank(c Confidence) int {
	switch c {
	case ConfidenceDetected:
		return 3
	case ConfidenceSuggested:
		return 2
	case ConfidencePossible:
		return 1
	default:
		return 0
	}
}

func uniqueEvidence(items []Evidence) []Evidence {
	seen := map[string]struct{}{}
	result := make([]Evidence, 0, len(items))
	for _, item := range items {
		key := string(item.Kind) + " " + item.Path + " " + item.Detail
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Detail < result[j].Detail
	})
	return result
}

func uniqueSorted(items []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func sortResult(result *Result) {
	sort.Slice(result.Findings, func(i, j int) bool {
		if result.Findings[i].Capability != result.Findings[j].Capability {
			return result.Findings[i].Capability < result.Findings[j].Capability
		}
		return result.Findings[i].Name < result.Findings[j].Name
	})
	sort.Slice(result.Ports, func(i, j int) bool {
		if result.Ports[i].Path != result.Ports[j].Path {
			return result.Ports[i].Path < result.Ports[j].Path
		}
		if result.Ports[i].Service != result.Ports[j].Service {
			return result.Ports[i].Service < result.Ports[j].Service
		}
		return result.Ports[i].Value < result.Ports[j].Value
	})
	result.HealthChecks = uniqueEvidence(result.HealthChecks)
}

func slugify(value string) string {
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

// MarshalJSONResult exists so control surfaces can share one stable rendering
// path without each re-encoding repository inspection state differently.
func MarshalJSONResult(result Result) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

// ParsePublishedPort extracts the host-side numeric port from common Compose
// short syntax. It is intentionally conservative and returns false on ambiguity.
func ParsePublishedPort(value string) (int, bool) {
	value = strings.TrimSpace(strings.Trim(value, ""'"))
	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return 0, false
	}
	host := parts[len(parts)-2]
	if strings.Contains(host, "-") {
		return 0, false
	}
	port, err := strconv.Atoi(host)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}
