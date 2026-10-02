package repositoryinspect

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxInspectionFileSize   = 2 << 20
	maxInspectionFiles      = 10000
	maxInspectionTotalBytes = 64 << 20
	maxInspectionDepth      = 64
)

type inspectionBudget struct {
	files int
	bytes int64
}

func (b *inspectionBudget) add(size int64) error {
	b.files++
	if b.files > maxInspectionFiles {
		return fmt.Errorf("repository inspection file limit exceeded: more than %d relevant files", maxInspectionFiles)
	}
	b.bytes += size
	if b.bytes > maxInspectionTotalBytes {
		return fmt.Errorf("repository inspection size limit exceeded: more than %d bytes of relevant files", maxInspectionTotalBytes)
	}
	return nil
}

var ignoredDirectories = map[string]struct{}{
	".git": {}, ".baseharbor": {}, "node_modules": {}, "vendor": {},
	"dist": {}, "build": {}, ".venv": {}, "venv": {}, "__pycache__": {},
}

func DefaultEngine() Engine {
	return Engine{Detectors: []Detector{
		sqlDetector{},
		keyValueDetector{},
		documentDatabaseDetector{},
		secretsDetector{},
		httpExposureDetector{},
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

	workloadCandidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		return Result{}, fmt.Errorf("inspect workload sources: %w", err)
	}
	result.WorkloadSourceCandidates = workloadCandidates
	repositoryMetadata, metadataErr := repositoryMetadataFromSnapshot(snapshot)
	if metadataErr != nil {
		result.WorkloadSourceResolution = WorkloadSourceResolution{
			SchemaVersion:  "baseharbor.workload-source-resolution/v1",
			State:          WorkloadSourceResolutionInvalid,
			Reason:         WorkloadSourceReasonInvalidRepositoryMetadata,
			CandidateCount: len(workloadCandidates),
			Message:        metadataErr.Error(),
		}
	} else {
		var explicit *WorkloadSourceCandidate
		if repositoryMetadata != nil {
			explicit = &WorkloadSourceCandidate{
				Kind: repositoryMetadata.WorkloadSource.Kind,
				Path: repositoryMetadata.WorkloadSource.Path,
			}
		}
		result.WorkloadSourceResolution = ResolveWorkloadSource(workloadCandidates, explicit)
		if result.WorkloadSourceResolution.State == WorkloadSourceResolutionInvalid && result.WorkloadSourceResolution.Message == "" {
			result.WorkloadSourceResolution.Message = fmt.Sprintf("%s does not match any detected workload source", RepositoryMetadataName)
		}
	}
	selected := result.WorkloadSourceResolution.Selected
	if selected != nil {
		result.SelectedWorkloadSource = selected
		evidence, normalizeErr := NormalizeWorkloadSource(snapshot, *selected)
		if normalizeErr != nil {
			return Result{}, fmt.Errorf("normalize workload source %s %s: %w", selected.Kind, selected.Path, normalizeErr)
		}
		result.WorkloadEvidence = &evidence
	}

	var manifest *application.Manifest
	var declared []CapabilityIntent
	if _, ok := snapshot.Files["baseharbor.yaml"]; ok {
		result.ExistingManifest = "baseharbor.yaml"
		loaded, err := application.LoadManifestFile(filepath.Join(absRoot, application.RepositoryManifestName))
		if err != nil {
			return Result{}, fmt.Errorf("load existing BaseHarbor manifest: %w", err)
		}
		manifest = &loaded
		result.Application = loaded.Name
		result.RequiredSecrets = append([]string(nil), application.RequiredSecretNames(loaded)...)
		declared, err = CapabilityIntentsFromManifest(loaded)
		if err != nil {
			return Result{}, fmt.Errorf("normalize existing BaseHarbor manifest intent: %w", err)
		}
		manifestEvidence := []Evidence{{Kind: EvidenceManifest, Path: application.RepositoryManifestName, Detail: "declared by BaseHarbor application contract"}}
		for _, intent := range declared {
			result.Findings = mergeFindings(result.Findings, []Finding{{
				Capability: intent.Capability,
				Name:       intent.Name,
				Direction:  intent.Direction,
				Confidence: ConfidenceDetected,
				Evidence:   manifestEvidence,
			}})
		}
	}
	result.ComposeCandidates = artifactPaths(artifacts, "compose")
	if manifest != nil {
		result.WorkloadServices = append([]string(nil), application.WorkloadComponentNames(*manifest)...)
	}
	projectWorkloadEvidenceSummary(&result, manifest == nil)
	if result.SelectedWorkloadSource != nil && result.SelectedWorkloadSource.Kind == WorkloadSourceCompose {
		result.SelectedCompose = result.SelectedWorkloadSource.Path
	}
	if result.SelectedCompose != "" {
		rel := result.SelectedCompose
		services, detectErr := detectComposeServices(snapshot.Files[rel])
		if detectErr != nil {
			return Result{}, fmt.Errorf("inspect selected Compose file %s: %w", rel, detectErr)
		}
		for _, service := range services {
			if manifest == nil && (service.AmbiguousInfrastructure || service.Unresolved) {
				result.AmbiguousServices = append(result.AmbiguousServices, service.Name)
			}
			if service.DatabaseBootstrap {
				result.DatabaseBootstrapServices = append(result.DatabaseBootstrapServices, service.Name)
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
	result.DatabaseBootstrapServices = uniqueSorted(result.DatabaseBootstrapServices)
	result.SecretCandidates = uniqueSorted(result.SecretCandidates)
	result.WorkloadServices = uniqueSorted(result.WorkloadServices)
	result.InfrastructureServices = uniqueSorted(result.InfrastructureServices)
	result.AmbiguousServices = uniqueSorted(result.AmbiguousServices)
	detectorSnapshot := snapshotForSelectedWorkloadSource(snapshot, result.SelectedWorkloadSource)
	for _, detector := range e.Detectors {
		if detector == nil {
			continue
		}
		findings, err := detector.Detect(ctx, detectorSnapshot)
		if err != nil {
			return Result{}, fmt.Errorf("repository detector %s: %w", detector.Name(), err)
		}
		result.Findings = mergeFindings(result.Findings, findings)
	}
	result.Findings = enrichDeclaredIntentEvidence(snapshot, declared, result.Findings)
	for i := range result.Findings {
		normalizeFindingService(&result.Findings[i])
	}
	result.Declared, result.Reconciliation = Reconcile(result.Findings, declared)
	sortResult(&result)
	return result, nil
}

func projectWorkloadEvidenceSummary(result *Result, classifyComponents bool) {
	if result.WorkloadEvidence == nil {
		return
	}
	evidence := result.WorkloadEvidence
	hasWorkload := false
	for _, component := range evidence.Components {
		path := workloadComponentEvidencePath(component, evidence.Source.Path)
		for _, port := range component.Ports {
			result.Ports = append(result.Ports, PortEvidence{
				Path: path, Service: component.ID, Value: port,
			})
		}
		if component.Health {
			result.HealthChecks = append(result.HealthChecks, Evidence{
				Kind:   EvidenceHealth,
				Path:   path,
				Detail: "workload component " + component.ID + " declares health/readiness",
			})
		}
		if !classifyComponents {
			continue
		}
		if component.InfrastructureClass != "" {
			result.InfrastructureServices = append(result.InfrastructureServices, component.ID)
			if capability := workloadInfrastructureCapability(component.InfrastructureClass); capability != "" {
				result.Findings = mergeFindings(result.Findings, []Finding{{
					Capability: capability,
					Name:       component.ID,
					Confidence: ConfidenceDetected,
					Evidence: []Evidence{{
						Kind:   workloadSourceEvidenceKind(evidence.Source.Kind),
						Path:   path,
						Detail: "normalized workload evidence classifies component " + component.ID + " as " + component.InfrastructureClass,
					}},
				}})
			}
			continue
		}
		result.WorkloadServices = append(result.WorkloadServices, component.ID)
		hasWorkload = true
	}
	if classifyComponents && hasWorkload {
		result.Findings = mergeFindings(result.Findings, []Finding{{
			Capability: "logs",
			Direction:  DirectionExport,
			Confidence: ConfidenceSuggested,
			Evidence: []Evidence{{
				Kind:   workloadSourceEvidenceKind(evidence.Source.Kind),
				Path:   evidence.Source.Path,
				Detail: "application workload can opt into managed stdout/stderr log collection",
			}},
		}})
	}
}

func workloadComponentEvidencePath(component WorkloadComponent, fallback string) string {
	for _, source := range component.Source {
		if strings.TrimSpace(source.Path) != "" {
			return source.Path
		}
	}
	return fallback
}

func workloadSourceEvidenceKind(kind WorkloadSourceKind) EvidenceKind {
	if kind == WorkloadSourceCompose {
		return EvidenceCompose
	}
	return EvidenceConfig
}

func workloadInfrastructureCapability(class string) string {
	switch class {
	case "database.sql", "cache.key-value", "database.document":
		return class
	default:
		return ""
	}
}

func snapshotForSelectedWorkloadSource(snapshot Snapshot, selected *WorkloadSourceCandidate) Snapshot {
	filtered := Snapshot{Root: snapshot.Root, Files: make(map[string][]byte, len(snapshot.Files))}
	selectedCompose := ""
	if selected != nil && selected.Kind == WorkloadSourceCompose {
		selectedCompose = filepath.ToSlash(selected.Path)
	}
	for path, data := range snapshot.Files {
		base := strings.ToLower(filepath.Base(path))
		if isComposeFile(base) && filepath.ToSlash(path) != selectedCompose {
			continue
		}
		filtered.Files[path] = data
	}
	return filtered
}

func singleRootComposeCandidate(candidates []string) string {
	var selected string
	for _, candidate := range candidates {
		candidate = filepath.ToSlash(strings.TrimSpace(candidate))
		if candidate == "" || strings.Contains(candidate, "/") {
			continue
		}
		if selected != "" {
			return ""
		}
		selected = candidate
	}
	return selected
}

func collectSnapshot(ctx context.Context, root string) (Snapshot, []Artifact, error) {
	snapshot := Snapshot{Root: root, Files: map[string][]byte{}}
	var artifacts []Artifact
	var budget inspectionBudget
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
		if len(strings.Split(filepath.ToSlash(rel), "/")) > maxInspectionDepth {
			if entry.IsDir() {
				return filepath.SkipDir
			}
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
		if err := budget.add(info.Size()); err != nil {
			return err
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
	case RepositoryMetadataName:
		return "baseharbor-repository-metadata", true
	case "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml":
		return "compose", true
	case "dockerfile":
		return "dockerfile", true
	case ".env.example", ".env.template", ".env.sample", ".env":
		return "env", true
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"go.mod", "go.sum", "pyproject.toml", "requirements.txt", "poetry.lock",
		"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts",
		"gradle.properties", "cargo.toml", "cargo.lock":
		return "dependency", true
	}
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".container", ".pod", ".network", ".volume", ".kube":
		return "quadlet", true
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".conf", ".properties":
		return "config", true
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".java", ".kt", ".kts":
		return "", true
	default:
		return "", false
	}
}
