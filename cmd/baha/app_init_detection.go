package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

func detectAppProject(root string) (appProjectDetection, error) {
	result, err := repositoryinspect.Inspect(context.Background(), root)
	if err != nil {
		return appProjectDetection{}, err
	}
	if result.WorkloadSourceResolution.State == repositoryinspect.WorkloadSourceResolutionInvalid {
		detail := result.WorkloadSourceResolution.Message
		if strings.TrimSpace(detail) == "" {
			detail = string(result.WorkloadSourceResolution.Reason)
		}
		return appProjectDetection{}, usageError(
			"repository workload source selection is invalid",
			detail+"; fix or remove "+repositoryinspect.RepositoryMetadataName+" and run app init again.",
		)
	}
	d := appProjectDetection{
		Name:                     result.Application,
		WorkloadSourceCandidates: append([]repositoryinspect.WorkloadSourceCandidate(nil), result.WorkloadSourceCandidates...),
		SelectedWorkloadSource:   result.SelectedWorkloadSource,
		WorkloadEvidence:         result.WorkloadEvidence,
		ComposeCandidates:        append([]string(nil), result.ComposeCandidates...),
		Compose:                  result.SelectedCompose,
		WorkloadServices:         append([]string(nil), result.WorkloadServices...),
		InfrastructureServices:   append([]string(nil), result.InfrastructureServices...),
		AmbiguousServices:        append([]string(nil), result.AmbiguousServices...),
		Ports:                    append([]repositoryinspect.PortEvidence(nil), result.Ports...),
		SecretCandidates:         append([]string(nil), result.SecretCandidates...),
		SecretSources:            map[string]string{},
		SecretSourcePolicies:     map[string]string{},
		RuntimePermissions:       map[string][]string{},
	}
	if result.WorkloadEvidence != nil {
		var workload []string
		var infrastructure []string
		d.Ports = nil
		for _, component := range result.WorkloadEvidence.Components {
			if component.InfrastructureClass != "" {
				infrastructure = append(infrastructure, component.ID)
				continue
			}
			workload = append(workload, component.ID)
			for _, port := range component.Ports {
				d.Ports = append(d.Ports, repositoryinspect.PortEvidence{Path: result.WorkloadEvidence.Source.Path, Service: component.ID, Value: port})
			}
		}
		if len(workload) > 0 {
			d.WorkloadServices = uniqueSorted(workload)
		}
		if len(infrastructure) > 0 {
			d.InfrastructureServices = uniqueSorted(infrastructure)
		}
		d.Ports = uniquePortEvidence(d.Ports)
	}
	if d.Compose != "" {
		analysis, err := repositoryinspect.AnalyzeComposeFile(root, d.Compose)
		if err != nil {
			return appProjectDetection{}, err
		}
		d.WorkloadProtocols = analysis.WorkloadProtocols
		policies, err := repositoryinspect.ReadComposeSecretSourcePolicies(root, d.Compose)
		if err != nil {
			return appProjectDetection{}, err
		}
		for name, source := range policies {
			d.SecretSourcePolicies[name] = source
			d.SecretCandidates = append(d.SecretCandidates, name)
			d.SecretSources[name] = d.Compose + " (BaseHarbor-managed source)"
		}
		d.SecretCandidates = uniqueSorted(d.SecretCandidates)
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
			d.SQL = true
			if finding.Name != "" {
				d.SQLInstances = append(d.SQLInstances, finding.Name)
			}
			if d.SQLSource == "" {
				d.SQLSource = source
			}
		case "cache.key-value":
			if !detected {
				continue
			}
			d.Cache = true
			if finding.Name != "" {
				d.CacheInstances = append(d.CacheInstances, finding.Name)
			}
			if d.CacheSource == "" {
				d.CacheSource = source
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
				// Record the repository module the proven metrics endpoint
				// lives in (the first space-delimited token of the source),
				// so the exporter can be tied to the workload that serves it.
				if mod := strings.Fields(source); len(mod) > 0 {
					d.MetricsSourceModule = mod[0]
				}
				// Carry the proven exact HTTP path (for example /q/metrics,
				// /actuator/prometheus, /metrics) so the quick/guided paths can
				// bind the detected endpoint tuple (source, service, internal
				// port, path) instead of hardcoding /metrics.
				if p, ok := metricsProvenPath(source); ok {
					d.MetricsPath = p
				}
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
	d.SQLInstances = uniqueSorted(d.SQLInstances)
	d.CacheInstances = uniqueSorted(d.CacheInstances)
	d.OTLPSignals = uniqueSorted(d.OTLPSignals)
	for capabilityID, operations := range d.RuntimePermissions {
		d.RuntimePermissions[capabilityID] = uniqueSorted(operations)
	}
	return d, nil
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

func uniquePortEvidence(input []repositoryinspect.PortEvidence) []repositoryinspect.PortEvidence {
	seen := map[string]struct{}{}
	out := make([]repositoryinspect.PortEvidence, 0, len(input))
	for _, item := range input {
		key := item.Path + "\x00" + item.Service + "\x00" + item.Value
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Service != out[j].Service {
			return out[i].Service < out[j].Service
		}
		if out[i].Value != out[j].Value {
			return out[i].Value < out[j].Value
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// metricsProvenPath extracts the proven exact HTTP metrics path from a
// detector source string ("<evidence path> <detail>"). The detail carries the
// classed path, for example "repository exposes a metrics endpoint /q/metrics
// (quarkus)". metricsProvenPathPattern provides the boundary-aware path
// tokenization; the token is then accepted ONLY by exact equality against the
// canonical registry of proven metrics endpoints (metricsProvenEndpoint).
// Arbitrary candidate paths such as "/health" or "/api/users" and
// prefix/substring variants are never proven here.
func metricsProvenPath(source string) (string, bool) {
	for _, matches := range metricsProvenPathPattern.FindAllStringSubmatch(source, -1) {
		if len(matches) < 2 {
			continue
		}
		path := matches[1]
		if metricsProvenEndpoint(path) {
			return path, true
		}
	}
	return "", false
}

// metricsProvenRegistry is the canonical registry of PROVEN metrics endpoint
// paths, mirroring the exact-match list maintained by the repositoryinspect
// detector (Quarkus /q/metrics, Spring /actuator/prometheus, conventional
// /metrics). Only these paths count as proven; everything else fails closed.
var metricsProvenRegistry = []string{"/q/metrics", "/actuator/prometheus", "/metrics"}

// metricsProvenEndpoint reports whether path is an exact match against the
// proven registry. Comparison is exact equality only: no prefix, substring or
// case-insensitive matching, so "/health", "/metrics-old" and "/api/users"
// are never proven.
func metricsProvenEndpoint(path string) bool {
	for _, proven := range metricsProvenRegistry {
		if path == proven {
			return true
		}
	}
	return false
}

// metricsProvenPathPattern captures an HTTP endpoint path preceded by a quote,
// equals, whitespace, or the start of the string. The CLI uses it to pull the
// first path-shaped token out of a detector evidence string. Endpoint
// provenance itself is decided ONLY by the registry comparison in
// metricsProvenEndpoint (exact equality), never by this pattern alone: the
// pattern is the boundary reference for well-formed tokens such as
// "/q/metrics" and must not match a bare substring inside unrelated text.
var metricsProvenPathPattern = regexp.MustCompile(`(?i)(?:["'=\s]|^)(/[A-Za-z0-9_./-]+)`)

// detectedMetricsTarget resolves the proven metrics endpoint tuple
// (source, service, internal port, HTTP path) for the sole selected workload.
// ok is false when detection is ambiguous (no single workload service, no
// workload exposing exactly one port, or the exporter's repository module is
// not attributable to that sole workload), so callers fail closed or ask
// instead of silently defaulting.
func detectedMetricsTarget(d appProjectDetection, workloadServices []string) (string, string, int, string, bool) {
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
		return "", "", 0, "", false
	}
	selectedService := targets[0].service
	// Tie the proven metrics endpoint to the sole workload. A proven exporter
	// (for example a Quarkus/Spring module detected in an unrelated module)
	// must not be mapped onto a lone workload (for example a Caddy sidecar)
	// that only happens to expose a port. Fail closed unless the exporter's
	// repository module can be attributed to that sole workload service.
	if !metricsModuleAttributableToWorkload(selectedService, workloadServices, d.MetricsSourceModule) {
		return "", "", 0, "", false
	}
	// Carry the proven exact HTTP path. Only registry paths are ever stored
	// in MetricsPath, and the endpoint evidence itself is proven only by the
	// registry (the detector emits no endpoint evidence for arbitrary
	// paths). Dependency or import evidence without any path token keeps
	// MetricsPath empty: the conventional /metrics endpoint is the documented
	// default there, never an arbitrary match.
	path := d.MetricsPath
	if path == "" {
		path = "/metrics"
	}
	if !metricsProvenEndpoint(path) {
		// Fail closed: never bind an unproven candidate path.
		return "", "", 0, "", false
	}
	return selectedService, selectedService, targets[0].port, path, true
}

// metricsModuleAttributableToWorkload reports whether the proven metrics
// endpoint (living in module `metricsModule`) can be unambiguously attributed
// to the sole selected workload service. It returns false unless exactly one
// workload service is selected, so a lone sidecar workload can never inherit
// an exporter path proven in an unrelated module.
func metricsModuleAttributableToWorkload(selectedService string, workloadServices []string, metricsModule string) bool {
	if len(workloadServices) != 1 {
		return false
	}
	if selectedService != workloadServices[0] {
		return false
	}
	if metricsModule == "" {
		// No proven exporter path: the conventional /metrics endpoint is
		// repository-global, so it is attributable to the sole workload.
		return true
	}
	// A proven exporter in a dedicated repository module (a path containing a
	// separator, for example "shared/metrics.go" or "backend/Main.java") that
	// is not the selected workload's own source must not be mapped onto a lone
	// workload (for example a Caddy sidecar) that only happens to expose a
	// port. A module living at the repository root (no separator) is the sole
	// workload's own code and is attributable. This is the exporter-to-
	// workload association guard: fail closed unless the exporter's module can
	// be tied to the sole selected workload service.
	if strings.ContainsRune(metricsModule, '/') {
		// Path-boundary match, not a substring match: "api" must match the
		// compose service "api" exactly, not "api-old" or any service that
		// merely contains the selected name as a prefix. Trim a single
		// trailing ".go" before comparing so a service-named module is
		// attributed correctly.
		module := strings.TrimSuffix(metricsModule, ".go")
		if module == selectedService {
			return true
		}
		// The module's compose-key component (its top directory, or the path
		// itself when unqualified) must equal the service so that "api" does
		// not leak onto "api-old".
		head := module
		if i := strings.IndexByte(module, '/'); i >= 0 {
			head = module[:i]
		}
		return head == selectedService
	}
	return true
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

func quickNamedInstances(detected []string) []string {
	detected = uniqueSorted(detected)
	if len(detected) > 1 {
		return detected
	}
	return nil
}
