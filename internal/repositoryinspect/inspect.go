package repositoryinspect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"github.com/mcpdev80/baseharbor/internal/application"
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
		objectStorageDetector{},
		openMetricsDetector{},
		otlpDetector{},
		runtimeAPIDetector{},
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
		ContractVersion: "v1",
		Root:            absRoot,
		Application:     slugify(filepath.Base(absRoot)),
		Artifacts:       artifacts,
		SecretSources:   map[string]string{},
	}
	if result.Application == "" {
		result.Application = "app"
	}

	var manifest *application.Manifest
	if _, ok := snapshot.Files["baseharbor.yaml"]; ok {
		result.ExistingManifest = "baseharbor.yaml"
		loaded, err := application.LoadManifestFile(filepath.Join(absRoot, application.RepositoryManifestName))
		if err != nil {
			return Result{}, fmt.Errorf("load existing BaseHarbor manifest: %w", err)
		}
		manifest = &loaded
		result.Application = loaded.Name
		result.RequiredSecrets = append([]string(nil), application.RequiredSecretNames(loaded)...)
		manifestEvidence := []Evidence{{Kind: EvidenceManifest, Path: application.RepositoryManifestName, Detail: "declared by BaseHarbor application contract"}}
		if loaded.Services.SQL {
			for _, name := range application.SQLInstanceNames(loaded) {
				result.Findings = mergeFindings(result.Findings, []Finding{{Capability: "database.sql", Name: name, Direction: DirectionConsume, Confidence: ConfidenceDetected, Evidence: manifestEvidence}})
			}
		}
		if loaded.Services.Cache {
			for _, name := range application.CacheInstanceNames(loaded) {
				result.Findings = mergeFindings(result.Findings, []Finding{{Capability: "cache.key-value", Name: name, Direction: DirectionConsume, Confidence: ConfidenceDetected, Evidence: manifestEvidence}})
			}
		}
		if loaded.Services.Secrets {
			result.Findings = mergeFindings(result.Findings, []Finding{{Capability: "secrets", Direction: DirectionConsume, Confidence: ConfidenceDetected, Evidence: manifestEvidence}})
		}
		for _, name := range application.ObjectStorageBucketNames(loaded) {
			result.Findings = mergeFindings(result.Findings, []Finding{{Capability: "object-storage.s3", Name: name, Direction: DirectionConsume, Confidence: ConfidenceDetected, Evidence: manifestEvidence}})
		}
		for _, exposure := range loaded.Exposures {
			result.Findings = mergeFindings(result.Findings, []Finding{{Capability: "exposure.http", Name: exposure.Name, Direction: DirectionProvide, Confidence: ConfidenceDetected, Evidence: manifestEvidence}})
		}
		if application.HasOTLPTelemetry(loaded) {
			result.Findings = mergeFindings(result.Findings, []Finding{{Capability: "telemetry.otlp", Name: "default", Direction: DirectionExport, Confidence: ConfidenceDetected, Evidence: manifestEvidence}})
		}
	}

	result.ComposeCandidates = artifactPaths(artifacts, "compose")
	if manifest != nil && strings.TrimSpace(manifest.Workload.Compose) != "" {
		result.SelectedCompose = filepath.ToSlash(manifest.Workload.Compose)
		result.WorkloadServices = append([]string(nil), manifest.Workload.Services...)
	} else if len(result.ComposeCandidates) == 1 {
		result.SelectedCompose = result.ComposeCandidates[0]
	}
	for _, rel := range result.ComposeCandidates {
		services := detectComposeServices(snapshot.Files[rel])
		for _, service := range services {
			if manifest == nil && rel == result.SelectedCompose {
				if service.Postgres || service.Redis || service.ObjectStorage {
					result.InfrastructureServices = append(result.InfrastructureServices, service.Name)
				} else if service.AmbiguousInfrastructure {
					result.AmbiguousServices = append(result.AmbiguousServices, service.Name)
				} else if service.HasBuild || service.HasImage || service.HasPorts {
					result.WorkloadServices = append(result.WorkloadServices, service.Name)
				}
			}
			for _, port := range service.Ports {
				result.Ports = append(result.Ports, PortEvidence{
					Path: rel, Service: service.Name, Value: port,
				})
			}
			if service.HealthCheck {
				result.HealthChecks = append(result.HealthChecks, Evidence{
					Kind: EvidenceHealth, Path: rel,
					Detail: "compose service " + service.Name + " declares healthcheck",
				})
			}
		}
	}
	for _, rel := range artifactPaths(artifacts, "dockerfile") {
		ports, health := inspectDockerfile(snapshot.Files[rel], rel)
		result.Ports = append(result.Ports, ports...)
		result.HealthChecks = append(result.HealthChecks, health...)
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
	result.InfrastructureServices = uniqueSorted(result.InfrastructureServices)
	result.AmbiguousServices = uniqueSorted(result.AmbiguousServices)
	if manifest == nil && result.SelectedCompose != "" && len(result.WorkloadServices) > 0 {
		result.Findings = mergeFindings(result.Findings, []Finding{{
			Capability: "logs",
			Direction:  DirectionExport,
			Confidence: ConfidenceSuggested,
			Evidence: []Evidence{{
				Kind:   EvidenceCompose,
				Path:   result.SelectedCompose,
				Detail: "application workload can opt into managed stdout/stderr log collection",
			}},
		}})
	}

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
	for i := range result.Findings {
		normalizeFindingService(&result.Findings[i])
	}
	result.Declared, result.Reconciliation = Reconcile(result.Findings, manifest)
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
		if entry.Type()&os.ModeSymlink != 0 {
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
		if kind == "env" {
			data = envNamesOnly(data)
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
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".conf", ".properties":
		return "config", true
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx":
		return "", true
	default:
		return "", false
	}
}
