package repositoryinspect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type WorkloadSourceKind string

const (
	WorkloadSourceCompose    WorkloadSourceKind = "compose"
	WorkloadSourceQuadlet    WorkloadSourceKind = "quadlet"
	WorkloadSourceKubernetes WorkloadSourceKind = "kubernetes"
)

const WorkloadSourceAdapterVersion = "baseharbor.workload-source-adapter/v1"

const maxKubernetesDocumentsPerFile = 2048

type WorkloadSourceAdapter interface {
	Version() string
	Kind() WorkloadSourceKind
	Detect(Snapshot) ([]WorkloadSourceCandidate, error)
	Resolve(Snapshot, WorkloadSourceCandidate) (WorkloadSourceCandidate, error)
	Normalize(Snapshot, WorkloadSourceCandidate) ([]WorkloadComponent, []WorkloadSourceReference, error)
	Fingerprint(Snapshot, WorkloadSourceCandidate) string
}

type composeWorkloadSourceAdapter struct{}
type quadletWorkloadSourceAdapter struct{}
type kubernetesWorkloadSourceAdapter struct{}

var workloadSourceAdapters = []WorkloadSourceAdapter{
	composeWorkloadSourceAdapter{},
	quadletWorkloadSourceAdapter{},
	kubernetesWorkloadSourceAdapter{},
}

func (composeWorkloadSourceAdapter) Version() string             { return WorkloadSourceAdapterVersion }
func (composeWorkloadSourceAdapter) Kind() WorkloadSourceKind    { return WorkloadSourceCompose }
func (quadletWorkloadSourceAdapter) Version() string             { return WorkloadSourceAdapterVersion }
func (quadletWorkloadSourceAdapter) Kind() WorkloadSourceKind    { return WorkloadSourceQuadlet }
func (kubernetesWorkloadSourceAdapter) Version() string          { return WorkloadSourceAdapterVersion }
func (kubernetesWorkloadSourceAdapter) Kind() WorkloadSourceKind { return WorkloadSourceKubernetes }

func workloadSourceAdapter(kind WorkloadSourceKind) (WorkloadSourceAdapter, error) {
	for _, adapter := range workloadSourceAdapters {
		if adapter.Kind() == kind {
			return adapter, nil
		}
	}
	return nil, fmt.Errorf("unsupported workload source kind %q", kind)
}

type WorkloadSourceCandidate struct {
	Kind       WorkloadSourceKind `json:"kind"`
	Path       string             `json:"path"`
	Confidence Confidence         `json:"confidence"`
	Evidence   []string           `json:"evidence,omitempty"`
	Unresolved []string           `json:"unresolved,omitempty"`
}

type WorkloadSourceReference struct {
	Kind     WorkloadSourceKind `json:"kind"`
	Path     string             `json:"path"`
	Resource string             `json:"resource,omitempty"`
}

type WorkloadComponent struct {
	ID                  string                    `json:"id"`
	Source              []WorkloadSourceReference `json:"source"`
	Image               string                    `json:"image,omitempty"`
	Build               string                    `json:"build,omitempty"`
	Ports               []string                  `json:"ports,omitempty"`
	Health              bool                      `json:"health,omitempty"`
	EnvironmentRefs     []string                  `json:"environment_refs,omitempty"`
	ConfigRefs          []string                  `json:"config_refs,omitempty"`
	Dependencies        []string                  `json:"dependencies,omitempty"`
	PersistentStorage   []string                  `json:"persistent_storage,omitempty"`
	Exposure            []string                  `json:"exposure,omitempty"`
	InfrastructureClass string                    `json:"infrastructure_class,omitempty"`
}

type WorkloadEvidence struct {
	SchemaVersion string                    `json:"schema_version"`
	Source        WorkloadSourceCandidate   `json:"source"`
	Components    []WorkloadComponent       `json:"components,omitempty"`
	Opaque        []WorkloadSourceReference `json:"opaque,omitempty"`
	Fingerprint   string                    `json:"fingerprint"`
}

type WorkloadSourceResolutionState string

const (
	WorkloadSourceResolutionSelected    WorkloadSourceResolutionState = "selected"
	WorkloadSourceResolutionAmbiguous   WorkloadSourceResolutionState = "ambiguous"
	WorkloadSourceResolutionNotDetected WorkloadSourceResolutionState = "not_detected"
	WorkloadSourceResolutionInvalid     WorkloadSourceResolutionState = "invalid"
	WorkloadSourceResolutionUnsupported WorkloadSourceResolutionState = "unsupported"
)

type WorkloadSourceResolutionReason string

const (
	WorkloadSourceReasonSingleCandidate               WorkloadSourceResolutionReason = "single_candidate"
	WorkloadSourceReasonExplicitRepositorySelection   WorkloadSourceResolutionReason = "explicit_repository_selection"
	WorkloadSourceReasonProductionCandidateDominates  WorkloadSourceResolutionReason = "production_candidate_dominates"
	WorkloadSourceReasonMultipleViableCandidates      WorkloadSourceResolutionReason = "multiple_viable_candidates"
	WorkloadSourceReasonCrossFamilyAmbiguity          WorkloadSourceResolutionReason = "cross_family_ambiguity"
	WorkloadSourceReasonOnlyLowConfidenceCandidates   WorkloadSourceResolutionReason = "only_low_confidence_candidates"
	WorkloadSourceReasonNoSupportedSource             WorkloadSourceResolutionReason = "no_supported_source"
	WorkloadSourceReasonInvalidRepositoryMetadata     WorkloadSourceResolutionReason = "invalid_repository_metadata"
	WorkloadSourceReasonUnsupportedRepositoryMetadata WorkloadSourceResolutionReason = "unsupported_repository_metadata"
)

type WorkloadSourceResolution struct {
	SchemaVersion  string                         `json:"schema_version"`
	State          WorkloadSourceResolutionState  `json:"state"`
	Reason         WorkloadSourceResolutionReason `json:"reason"`
	Selected       *WorkloadSourceCandidate       `json:"selected,omitempty"`
	CandidateCount int                            `json:"candidate_count"`
	Message        string                         `json:"message,omitempty"`
}

func InspectWorkloadSources(snapshot Snapshot) ([]WorkloadSourceCandidate, error) {
	var candidates []WorkloadSourceCandidate
	for _, adapter := range workloadSourceAdapters {
		detected, err := adapter.Detect(snapshot)
		if err != nil {
			return nil, fmt.Errorf("detect %s workload source: %w", adapter.Kind(), err)
		}
		candidates = append(candidates, detected...)
	}
	for i := range candidates {
		candidates[i].Evidence = uniqueSorted(candidates[i].Evidence)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Kind != candidates[j].Kind {
			return candidates[i].Kind < candidates[j].Kind
		}
		return candidates[i].Path < candidates[j].Path
	})
	return candidates, nil
}

func (composeWorkloadSourceAdapter) Detect(snapshot Snapshot) ([]WorkloadSourceCandidate, error) {
	var candidates []WorkloadSourceCandidate
	for path := range snapshot.Files {
		if !isComposeSourceFile(strings.ToLower(filepath.Base(path))) {
			continue
		}
		candidates = append(candidates, WorkloadSourceCandidate{
			Kind: WorkloadSourceCompose, Path: filepath.ToSlash(path),
			Confidence: ConfidenceDetected, Evidence: []string{filepath.ToSlash(path)},
		})
	}
	return candidates, nil
}

func (quadletWorkloadSourceAdapter) Detect(snapshot Snapshot) ([]WorkloadSourceCandidate, error) {
	var candidates []WorkloadSourceCandidate
	for path, data := range snapshot.Files {
		if !isQuadletFile(strings.ToLower(filepath.Base(path))) {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir == "." {
			dir = "."
		}
		evidence := []string{filepath.ToSlash(path)}
		var unresolved []string
		if strings.EqualFold(filepath.Ext(path), ".kube") {
			values := parseINIValues(string(data))
			rawRef := firstINIValue(values, "kube.yaml")
			if ref := resolveQuadletLocalReference(path, rawRef); ref != "" {
				if _, ok := snapshot.Files[ref]; ok {
					evidence = append(evidence, ref)
				} else {
					unresolved = append(unresolved, ref)
				}
			} else if strings.TrimSpace(rawRef) != "" && !strings.Contains(rawRef, "://") {
				unresolved = append(unresolved, rawRef)
			}
		}
		candidates = appendOrMergeCandidate(candidates, WorkloadSourceCandidate{
			Kind: WorkloadSourceQuadlet, Path: dir,
			Confidence: ConfidenceDetected, Evidence: evidence, Unresolved: unresolved,
		})
	}
	return candidates, nil
}

func (kubernetesWorkloadSourceAdapter) Detect(snapshot Snapshot) ([]WorkloadSourceCandidate, error) {
	var candidates []WorkloadSourceCandidate
	embedded := quadletEmbeddedKubernetesPaths(snapshot)
	for path, data := range snapshot.Files {
		path = filepath.ToSlash(path)
		if _, ok := embedded[path]; ok {
			continue
		}
		if !looksLikeKubernetesYAML(path, data) {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir == "." {
			dir = "."
		}
		candidates = appendOrMergeCandidate(candidates, WorkloadSourceCandidate{
			Kind: WorkloadSourceKubernetes, Path: dir,
			Confidence: ConfidenceDetected, Evidence: []string{path},
		})
	}
	return candidates, nil
}

func quadletEmbeddedKubernetesPaths(snapshot Snapshot) map[string]struct{} {
	result := map[string]struct{}{}
	for path, data := range snapshot.Files {
		if !strings.EqualFold(filepath.Ext(path), ".kube") {
			continue
		}
		values := parseINIValues(string(data))
		ref := resolveQuadletLocalReference(path, firstINIValue(values, "kube.yaml"))
		if ref == "" {
			continue
		}
		if _, ok := snapshot.Files[ref]; ok {
			result[ref] = struct{}{}
		}
	}
	return result
}

func resolveQuadletLocalReference(unitPath, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "://") || filepath.IsAbs(ref) {
		return ""
	}
	resolved := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(unitPath), ref)))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return ""
	}
	return resolved
}

func NormalizeRepositoryWorkloadSource(root string, candidate WorkloadSourceCandidate) (WorkloadEvidence, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return WorkloadEvidence{}, err
	}
	snapshot, _, err := collectSnapshot(context.Background(), absRoot)
	if err != nil {
		return WorkloadEvidence{}, err
	}
	return NormalizeWorkloadSource(snapshot, candidate)
}

func NormalizeWorkloadSource(snapshot Snapshot, candidate WorkloadSourceCandidate) (WorkloadEvidence, error) {
	adapter, err := workloadSourceAdapter(candidate.Kind)
	if err != nil {
		return WorkloadEvidence{}, err
	}
	resolved, err := adapter.Resolve(snapshot, candidate)
	if err != nil {
		return WorkloadEvidence{}, err
	}
	components, opaque, err := adapter.Normalize(snapshot, resolved)
	if err != nil {
		return WorkloadEvidence{}, err
	}
	sort.Slice(components, func(i, j int) bool { return components[i].ID < components[j].ID })
	return WorkloadEvidence{
		SchemaVersion: "baseharbor.workload-evidence/v1",
		Source:        resolved,
		Components:    components,
		Opaque:        opaque,
		Fingerprint:   adapter.Fingerprint(snapshot, resolved),
	}, nil
}

func resolveDetectedCandidate(snapshot Snapshot, candidate WorkloadSourceCandidate, kind WorkloadSourceKind) (WorkloadSourceCandidate, error) {
	if candidate.Kind != kind {
		return WorkloadSourceCandidate{}, fmt.Errorf("workload source adapter %s cannot resolve %s candidate", kind, candidate.Kind)
	}
	if len(candidate.Evidence) == 0 {
		return WorkloadSourceCandidate{}, fmt.Errorf("%s workload source %s has no evidence", kind, candidate.Path)
	}
	for _, path := range candidate.Evidence {
		if _, ok := snapshot.Files[path]; !ok {
			return WorkloadSourceCandidate{}, fmt.Errorf("%s workload source evidence %s is missing", kind, path)
		}
	}
	candidate.Evidence = uniqueSorted(candidate.Evidence)
	return candidate, nil
}

func (composeWorkloadSourceAdapter) Resolve(snapshot Snapshot, candidate WorkloadSourceCandidate) (WorkloadSourceCandidate, error) {
	resolved, err := resolveDetectedCandidate(snapshot, candidate, WorkloadSourceCompose)
	if err != nil {
		return WorkloadSourceCandidate{}, err
	}
	data := snapshot.Files[resolved.Path]
	var document composeDocument
	if err := yaml.Unmarshal(data, &document); err != nil {
		return WorkloadSourceCandidate{}, fmt.Errorf("decode Compose YAML: %w", err)
	}
	for service, definition := range document.Services {
		for _, key := range []string{"image", "build", "ports"} {
			raw, ok := definition[key]
			if !ok || raw == nil {
				continue
			}
			var values []string
			if key == "ports" {
				values = composePortValues(raw)
			} else {
				values = []string{strings.TrimSpace(fmt.Sprint(raw))}
			}
			for _, value := range values {
				value = resolveComposeDeterministicDefaults(value)
				for _, ref := range composeInterpolationReferences(value) {
					resolved.Unresolved = append(resolved.Unresolved, "services."+service+"."+key+" "+ref)
				}
			}
		}
	}
	resolved.Unresolved = uniqueSorted(resolved.Unresolved)
	return resolved, nil
}
func (quadletWorkloadSourceAdapter) Resolve(snapshot Snapshot, candidate WorkloadSourceCandidate) (WorkloadSourceCandidate, error) {
	resolved, err := resolveDetectedCandidate(snapshot, candidate, WorkloadSourceQuadlet)
	if err != nil {
		return WorkloadSourceCandidate{}, err
	}
	if len(resolved.Unresolved) > 0 {
		return WorkloadSourceCandidate{}, fmt.Errorf("quadlet workload source %s has unresolved inputs: %s", resolved.Path, strings.Join(uniqueSorted(resolved.Unresolved), ", "))
	}
	return resolved, nil
}
func (kubernetesWorkloadSourceAdapter) Resolve(snapshot Snapshot, candidate WorkloadSourceCandidate) (WorkloadSourceCandidate, error) {
	return resolveDetectedCandidate(snapshot, candidate, WorkloadSourceKubernetes)
}

func (composeWorkloadSourceAdapter) Normalize(snapshot Snapshot, candidate WorkloadSourceCandidate) ([]WorkloadComponent, []WorkloadSourceReference, error) {
	components, err := normalizeComposeSource(snapshot, candidate)
	return components, nil, err
}
func (quadletWorkloadSourceAdapter) Normalize(snapshot Snapshot, candidate WorkloadSourceCandidate) ([]WorkloadComponent, []WorkloadSourceReference, error) {
	components, err := normalizeQuadletSource(snapshot, candidate)
	return components, nil, err
}
func (kubernetesWorkloadSourceAdapter) Normalize(snapshot Snapshot, candidate WorkloadSourceCandidate) ([]WorkloadComponent, []WorkloadSourceReference, error) {
	return normalizeKubernetesSource(snapshot, candidate)
}

func (composeWorkloadSourceAdapter) Fingerprint(snapshot Snapshot, candidate WorkloadSourceCandidate) string {
	return fingerprintSource(snapshot, candidate)
}
func (quadletWorkloadSourceAdapter) Fingerprint(snapshot Snapshot, candidate WorkloadSourceCandidate) string {
	return fingerprintSource(snapshot, candidate)
}
func (kubernetesWorkloadSourceAdapter) Fingerprint(snapshot Snapshot, candidate WorkloadSourceCandidate) string {
	return fingerprintSource(snapshot, candidate)
}

func ResolveWorkloadSource(candidates []WorkloadSourceCandidate, explicit *WorkloadSourceCandidate) WorkloadSourceResolution {
	resolution := WorkloadSourceResolution{
		SchemaVersion:  "baseharbor.workload-source-resolution/v1",
		CandidateCount: len(candidates),
	}
	if explicit != nil {
		for i := range candidates {
			candidate := candidates[i]
			if candidate.Kind == explicit.Kind && filepath.ToSlash(candidate.Path) == filepath.ToSlash(explicit.Path) {
				selected := candidate
				resolution.State = WorkloadSourceResolutionSelected
				resolution.Reason = WorkloadSourceReasonExplicitRepositorySelection
				resolution.Selected = &selected
				return resolution
			}
		}
		resolution.State = WorkloadSourceResolutionInvalid
		resolution.Reason = WorkloadSourceReasonInvalidRepositoryMetadata
		return resolution
	}
	if len(candidates) == 0 {
		resolution.State = WorkloadSourceResolutionNotDetected
		resolution.Reason = WorkloadSourceReasonNoSupportedSource
		return resolution
	}
	if len(candidates) == 1 {
		selected := candidates[0]
		resolution.State = WorkloadSourceResolutionSelected
		resolution.Reason = WorkloadSourceReasonSingleCandidate
		resolution.Selected = &selected
		return resolution
	}

	kinds := map[WorkloadSourceKind]struct{}{}
	for _, candidate := range candidates {
		kinds[candidate.Kind] = struct{}{}
	}
	selected := selectWorkloadSource(candidates)
	if selected == nil {
		resolution.State = WorkloadSourceResolutionAmbiguous
		if len(kinds) > 1 {
			resolution.Reason = WorkloadSourceReasonCrossFamilyAmbiguity
		} else {
			allWeak := true
			for _, candidate := range candidates {
				if workloadSourceCandidateScore(candidate) >= 30 {
					allWeak = false
					break
				}
			}
			if allWeak {
				resolution.Reason = WorkloadSourceReasonOnlyLowConfidenceCandidates
			} else {
				resolution.Reason = WorkloadSourceReasonMultipleViableCandidates
			}
		}
		return resolution
	}
	resolution.State = WorkloadSourceResolutionSelected
	resolution.Reason = WorkloadSourceReasonProductionCandidateDominates
	resolution.Selected = selected
	return resolution
}

func selectWorkloadSource(candidates []WorkloadSourceCandidate) *WorkloadSourceCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		selected := candidates[0]
		return &selected
	}

	type scoredCandidate struct {
		candidate WorkloadSourceCandidate
		score     int
	}
	scored := make([]scoredCandidate, 0, len(candidates))
	kinds := map[WorkloadSourceKind]struct{}{}
	for _, candidate := range candidates {
		scored = append(scored, scoredCandidate{
			candidate: candidate,
			score:     workloadSourceCandidateScore(candidate),
		})
		kinds[candidate.Kind] = struct{}{}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].candidate.Kind != scored[j].candidate.Kind {
			return scored[i].candidate.Kind < scored[j].candidate.Kind
		}
		return scored[i].candidate.Path < scored[j].candidate.Path
	})
	if len(scored) < 2 || scored[0].score == scored[1].score {
		return nil
	}

	top := scored[0]
	if top.score < 30 {
		return nil
	}

	if len(kinds) > 1 {
		for _, other := range scored[1:] {
			if other.score > -40 {
				return nil
			}
		}
		selected := top.candidate
		return &selected
	}

	if top.score-scored[1].score < 30 {
		return nil
	}
	selected := top.candidate
	return &selected
}

func workloadSourceCandidateScore(candidate WorkloadSourceCandidate) int {
	path := filepath.ToSlash(filepath.Clean(strings.TrimSpace(candidate.Path)))
	if path == "" {
		path = "."
	}
	if path == "." || !strings.Contains(path, "/") {
		return 100
	}

	segments := strings.Split(strings.ToLower(path), "/")
	score := 0
	for i, segment := range segments {
		switch segment {
		case "docker", "deploy", "deployment", "deployments", "manifests", "manifest", "k8s", "kubernetes", "quadlet", "quadlets":
			if i <= 1 {
				score += 40
			} else {
				score += 20
			}
		case "infra", "infrastructure", "ops", "operations":
			if i <= 1 {
				score += 25
			} else {
				score += 10
			}
		case ".github", ".gitlab", ".agents", "test", "tests", "testing", "e2e", "example", "examples", "sample", "samples", "fixture", "fixtures", "sandbox":
			score -= 100
		case "docs", "doc", "documentation":
			score -= 70
		case "scripts", "script", "tools", "tooling":
			score -= 25
		}
	}
	return score
}

func appendOrMergeCandidate(candidates []WorkloadSourceCandidate, next WorkloadSourceCandidate) []WorkloadSourceCandidate {
	for i := range candidates {
		if candidates[i].Kind == next.Kind && candidates[i].Path == next.Path {
			candidates[i].Evidence = append(candidates[i].Evidence, next.Evidence...)
			candidates[i].Unresolved = append(candidates[i].Unresolved, next.Unresolved...)
			return candidates
		}
	}
	return append(candidates, next)
}

func fingerprintSource(snapshot Snapshot, candidate WorkloadSourceCandidate) string {
	h := sha256.New()
	paths := fingerprintSourcePaths(snapshot, candidate)
	for _, path := range paths {
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write(snapshot.Files[path])
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func fingerprintSourcePaths(snapshot Snapshot, candidate WorkloadSourceCandidate) []string {
	selected := map[string]struct{}{}
	add := func(path string) {
		path = filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
		if path == "" || path == "." || path == ".." || strings.HasPrefix(path, "../") {
			return
		}
		if _, ok := snapshot.Files[path]; ok {
			selected[path] = struct{}{}
		}
	}
	for _, path := range candidate.Evidence {
		add(path)
	}

	switch candidate.Kind {
	case WorkloadSourceCompose:
		data := snapshot.Files[candidate.Path]
		var document composeDocument
		if yaml.Unmarshal(data, &document) == nil {
			sourceDir := filepath.ToSlash(filepath.Dir(candidate.Path))
			if sourceDir == "." {
				sourceDir = ""
			}
			for _, definition := range document.Services {
				rawBuild, hasBuild := definition["build"]
				if !hasBuild || rawBuild == nil {
					continue
				}
				contextPath := "."
				dockerfile := "Dockerfile"
				switch build := rawBuild.(type) {
				case string:
					if strings.TrimSpace(build) != "" {
						contextPath = strings.TrimSpace(build)
					}
				case map[string]any:
					if value := strings.TrimSpace(fmt.Sprint(build["context"])); value != "" && value != "<nil>" {
						contextPath = value
					}
					if value := strings.TrimSpace(fmt.Sprint(build["dockerfile"])); value != "" && value != "<nil>" {
						dockerfile = value
					}
				}
				contextRoot := filepath.ToSlash(filepath.Clean(filepath.Join(sourceDir, contextPath)))
				if contextRoot == "." {
					contextRoot = ""
				}
				for path := range snapshot.Files {
					if path == RepositoryMetadataName || path == "baseharbor.yaml" {
						continue
					}
					if contextRoot == "" || path == contextRoot || strings.HasPrefix(path, contextRoot+"/") {
						selected[path] = struct{}{}
					}
				}
				add(filepath.Join(contextRoot, dockerfile))
				for _, ref := range composeReferenceValues(definition["env_file"]) {
					add(filepath.Join(sourceDir, ref))
				}
			}
		}
	case WorkloadSourceQuadlet:
		for _, unitPath := range candidate.Evidence {
			values := parseINIValues(string(snapshot.Files[unitPath]))
			unitDir := filepath.ToSlash(filepath.Dir(unitPath))
			if unitDir == "." {
				unitDir = ""
			}
			for _, ref := range append(append([]string(nil), values["container.environmentfile"]...), values["kube.yaml"]...) {
				ref = strings.TrimSpace(ref)
				if ref == "" || strings.Contains(ref, "://") {
					continue
				}
				add(filepath.Join(unitDir, ref))
			}
		}
	}

	paths := make([]string, 0, len(selected))
	for path := range selected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func classifyInfrastructure(name, image string) string {
	value := strings.ToLower(name + " " + image)
	switch {
	case strings.Contains(value, "postgres"), strings.Contains(value, "mysql"), strings.Contains(value, "mariadb"):
		return "database.sql"
	case strings.Contains(value, "redis"), strings.Contains(value, "valkey"):
		return "cache.key-value"
	case strings.Contains(value, "mongo"):
		return "database.document"
	case strings.Contains(value, "rabbitmq"):
		return "messaging"
	case strings.Contains(value, "minio"), strings.Contains(value, "seaweedfs"):
		return "storage.object"
	default:
		return ""
	}
}

func normalizeLogicalComponentID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func mergeComponents(input []WorkloadComponent) []WorkloadComponent {
	byID := map[string]WorkloadComponent{}
	for _, c := range input {
		if c.ID == "" {
			continue
		}
		current, ok := byID[c.ID]
		if !ok {
			byID[c.ID] = c
			continue
		}
		current.Source = append(current.Source, c.Source...)
		current.Ports = uniqueSorted(append(current.Ports, c.Ports...))
		current.EnvironmentRefs = uniqueSorted(append(current.EnvironmentRefs, c.EnvironmentRefs...))
		current.ConfigRefs = uniqueSorted(append(current.ConfigRefs, c.ConfigRefs...))
		current.Dependencies = uniqueSorted(append(current.Dependencies, c.Dependencies...))
		current.PersistentStorage = uniqueSorted(append(current.PersistentStorage, c.PersistentStorage...))
		current.Exposure = uniqueSorted(append(current.Exposure, c.Exposure...))
		current.Health = current.Health || c.Health
		if current.Image == "" {
			current.Image = c.Image
		}
		if current.Build == "" {
			current.Build = c.Build
		}
		if current.InfrastructureClass == "" {
			current.InfrastructureClass = c.InfrastructureClass
		}
		byID[c.ID] = current
	}
	out := make([]WorkloadComponent, 0, len(byID))
	for _, c := range byID {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func stringMapValue(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	value, ok := m[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func nestedMap(root map[string]any, keys ...string) map[string]any {
	var current any = root
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[key]
	}
	m, _ := current.(map[string]any)
	return m
}

func nestedString(root map[string]any, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	var current any = root
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[key]
	}
	if current == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(current))
}

func intLikeString(v any) string {
	switch x := v.(type) {
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float64:
		return strconv.Itoa(int(x))
	case string:
		return strings.TrimSpace(x)
	default:
		return ""
	}
}
