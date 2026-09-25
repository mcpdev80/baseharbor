package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

type guidedInitSelection struct {
	name                 string
	environment          string
	compose              string
	workloadServices     []string
	selected             []bool
	sqlInstances         []string
	cacheInstances       []string
	objectStorageBuckets []string
	secretPolicies       []guidedSecretPolicy
}

func collectGuidedInitSelection(reader *bufio.Reader, out io.Writer, d appProjectDetection) (guidedInitSelection, error) {
	var selection guidedInitSelection

	name, err := promptLine(reader, out, "Application name", d.Name)
	if err != nil {
		return selection, err
	}
	selection.name = slugifyAppName(name)
	if selection.name == "" {
		return selection, errors.New("application name cannot be empty")
	}

	environment, err := promptLine(reader, out, "Environment", "dev")
	if err != nil {
		return selection, err
	}
	selection.environment = slugifyAppName(environment)
	if selection.environment == "" {
		return selection, errors.New("environment cannot be empty")
	}

	selection.compose, selection.workloadServices, err = guidedWorkloadSelection(reader, out, d)
	if err != nil {
		return selection, err
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
	selection.selected, err = promptCapabilityList(reader, out, defaults, len(selection.workloadServices) > 0)
	if err != nil {
		return selection, err
	}

	if selection.selected[0] {
		selection.sqlInstances, err = promptServiceInstances(reader, out, "PostgreSQL", d.SQLInstances)
		if err != nil {
			return selection, err
		}
	}
	if selection.selected[1] {
		selection.cacheInstances, err = promptServiceInstances(reader, out, "Valkey / Redis", d.CacheInstances)
		if err != nil {
			return selection, err
		}
	}
	if selection.selected[2] {
		selection.objectStorageBuckets, err = promptServiceInstances(reader, out, "S3 buckets", nil)
		if err != nil {
			return selection, err
		}
		if len(selection.objectStorageBuckets) == 0 {
			selection.objectStorageBuckets = []string{"default"}
		}
	}
	if selection.selected[3] {
		printManagedCredentialSummary(out, selection.selected, len(d.RuntimePermissions) > 0)
		selection.secretPolicies, err = promptSecretPolicies(reader, out, d.SecretCandidates, d.SecretSources)
		if err != nil {
			return selection, err
		}
		additional, err := promptLine(reader, out, "Additional application secret names (comma-separated, Enter for none)", "")
		if err != nil {
			return selection, err
		}
		for _, item := range strings.Split(additional, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				selection.secretPolicies = append(selection.secretPolicies, guidedSecretPolicy{Name: item, Required: true, Provision: "later"})
			}
		}
	}

	return selection, nil
}

func guidedWorkloadSelection(reader *bufio.Reader, out io.Writer, d appProjectDetection) (string, []string, error) {
	compose := d.Compose
	workloadServices := append([]string(nil), d.WorkloadServices...)
	ambiguousServices := append([]string(nil), d.AmbiguousServices...)

	if len(d.ComposeCandidates) > 1 {
		var err error
		compose, err = promptCompose(reader, out, d.ComposeCandidates)
		if err != nil {
			return "", nil, err
		}
		analysis, err := repositoryinspect.AnalyzeComposeFile(".", compose)
		if err != nil {
			return "", nil, err
		}
		workloadServices = append([]string(nil), analysis.WorkloadServices...)
		ambiguousServices = append([]string(nil), analysis.AmbiguousServices...)
	}
	if len(ambiguousServices) > 0 {
		confirmedWorkload, err := promptAmbiguousComposeServices(reader, out, ambiguousServices)
		if err != nil {
			return "", nil, err
		}
		workloadServices = uniqueSorted(append(workloadServices, confirmedWorkload...))
	}
	return compose, workloadServices, nil
}

func buildGuidedInitManifest(reader *bufio.Reader, out io.Writer, d appProjectDetection, selection guidedInitSelection) (application.Manifest, error) {
	m := detectedApplicationManifest(
		selection.name,
		selection.environment,
		selection.selected[0],
		selection.selected[1],
		selection.selected[2],
		selection.selected[3],
		selection.compose != "" && len(selection.workloadServices) > 0,
	)
	if len(selection.sqlInstances) > 0 {
		m = application.WithSQLInstances(m, selection.sqlInstances...)
	}
	if len(selection.cacheInstances) > 0 {
		m = application.WithCacheInstances(m, selection.cacheInstances...)
	}
	if len(selection.objectStorageBuckets) > 0 {
		m = application.WithObjectStorageBuckets(m, selection.objectStorageBuckets...)
	}
	m = applyGuidedSecretPolicies(m, selection.secretPolicies)
	if selection.compose != "" && len(selection.workloadServices) > 0 {
		m = application.WithWorkload(m, filepath.ToSlash(selection.compose), selection.workloadServices...)
	}

	var err error
	m, err = addGuidedObservability(reader, out, d, selection, m)
	if err != nil {
		return application.Manifest{}, err
	}
	if len(d.RuntimePermissions) > 0 {
		services, err := runtimePermissionServices(reader, out, selection.workloadServices)
		if err != nil {
			return application.Manifest{}, err
		}
		for capabilityID, operations := range d.RuntimePermissions {
			m = application.WithRuntimePermission(m, capabilityID, services, operations...)
		}
	}
	return m, nil
}

func addGuidedObservability(reader *bufio.Reader, out io.Writer, d appProjectDetection, selection guidedInitSelection, m application.Manifest) (application.Manifest, error) {
	if selection.selected[4] {
		service, port, ok := detectedMetricsTarget(d, selection.workloadServices)
		if !ok {
			var err error
			service, port, err = promptMetricsTarget(reader, out, selection.workloadServices)
			if err != nil {
				return application.Manifest{}, err
			}
		}
		m = application.WithMetricsSource(m, "application", service, port, "/metrics")
	}
	if selection.selected[5] {
		defaultSignals := strings.Join(d.OTLPSignals, ",")
		if defaultSignals == "" {
			defaultSignals = "traces"
		}
		value, err := promptLine(reader, out, "OTLP signals (comma-separated)", defaultSignals)
		if err != nil {
			return application.Manifest{}, err
		}
		signals := uniqueSorted(strings.Split(value, ","))
		if len(signals) == 0 {
			return application.Manifest{}, errors.New("OTLP telemetry requires at least one signal")
		}
		m = application.WithOTLPTelemetry(m, signals...)
	}
	if selection.selected[6] {
		m = application.WithLogsCollection(m, "application")
	}
	return m, nil
}
