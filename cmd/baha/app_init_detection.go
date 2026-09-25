package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

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
