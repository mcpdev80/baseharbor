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
	name                         string
	environment                  string
	compose                      string
	workloadServices             []string
	workloadProtocols            map[string]string
	workloadPorts                []repositoryinspect.PortEvidence
	selected                     []bool
	sqlInstances                 []string
	cacheInstances               []string
	keyValueInstances            []string
	documentDatabaseInstances    []string
	messagingQueueInstances      []string
	messagingPubSubInstances     []string
	messagingStreamInstances     []string
	objectStorageBuckets         []string
	secretPolicies               []guidedSecretPolicy
	sqlManagementUI              bool
	cacheManagementUI            bool
	keyValueManagementUI         bool
	documentDatabaseManagementUI bool
	messagingManagementUI        bool
	objectStorageManagementUI    bool
	secretsManagementUI          bool
	identityManagementUI         bool
	observabilityManagementUI    bool
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

	selection.compose, selection.workloadServices, selection.workloadProtocols, selection.workloadPorts, err = guidedWorkloadSelection(reader, out, d)
	if err != nil {
		return selection, err
	}

	defaults := make([]bool, guidedCapabilityCount)
	defaults[guidedCapabilitySQL] = d.SQL
	defaults[guidedCapabilityCache] = d.Cache
	defaults[guidedCapabilityObjectStorage] = d.ObjectStorage
	defaults[guidedCapabilitySecrets] = len(d.SecretCandidates) > 0
	defaults[guidedCapabilityIdentity] = false
	defaults[guidedCapabilityMetrics] = d.Metrics
	defaults[guidedCapabilityOTLP] = d.OTLP && len(d.OTLPSignals) > 0
	defaults[guidedCapabilityLogs] = false
	selection.selected, err = promptCapabilityList(reader, out, defaults, len(selection.workloadServices) > 0)
	if err != nil {
		return selection, err
	}

	if selection.selected[guidedCapabilitySQL] {
		selection.sqlInstances, err = promptServiceInstances(reader, out, "PostgreSQL", d.SQLInstances)
		if err != nil {
			return selection, err
		}
	}
	if selection.selected[guidedCapabilityCache] {
		selection.cacheInstances, err = promptServiceInstances(reader, out, "Valkey / Redis cache", d.CacheInstances)
		if err != nil {
			return selection, err
		}
	}
	if selection.selected[guidedCapabilityDurableKeyValue] {
		selection.keyValueInstances, err = promptServiceInstances(reader, out, "Durable Valkey / Redis", nil)
		if err != nil {
			return selection, err
		}
	}
	if selection.selected[guidedCapabilityDocumentDatabase] {
		selection.documentDatabaseInstances, err = promptServiceInstances(reader, out, "MongoDB-compatible document database", nil)
		if err != nil {
			return selection, err
		}
	}
	if selection.selected[guidedCapabilityMessagingQueue] {
		selection.messagingQueueInstances, err = promptServiceInstances(reader, out, "Messaging queue", nil)
		if err != nil {
			return selection, err
		}
	}
	if selection.selected[guidedCapabilityMessagingPubSub] {
		selection.messagingPubSubInstances, err = promptServiceInstances(reader, out, "Messaging pub/sub", nil)
		if err != nil {
			return selection, err
		}
	}
	if selection.selected[guidedCapabilityMessagingStream] {
		selection.messagingStreamInstances, err = promptServiceInstances(reader, out, "Messaging stream", nil)
		if err != nil {
			return selection, err
		}
	}
	if selection.selected[guidedCapabilityObjectStorage] {
		selection.objectStorageBuckets, err = promptServiceInstances(reader, out, "S3 buckets", nil)
		if err != nil {
			return selection, err
		}
		if len(selection.objectStorageBuckets) == 0 {
			selection.objectStorageBuckets = []string{"default"}
		}
	}
	if selection.selected[guidedCapabilitySecrets] {
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

	if appInitReaderIsRealTerminal(appInitInput) {
		if selection.selected[guidedCapabilitySQL] {
			selection.sqlManagementUI, err = promptOptionalYesNo(reader, out, "PostgreSQL management UI?", false)
			if err != nil {
				return selection, err
			}
		}
		if selection.selected[guidedCapabilityCache] {
			selection.cacheManagementUI, err = promptOptionalYesNo(reader, out, "Cache management UI?", false)
			if err != nil {
				return selection, err
			}
		}
		if selection.selected[guidedCapabilityDurableKeyValue] {
			selection.keyValueManagementUI, err = promptOptionalYesNo(reader, out, "Durable key-value management UI?", false)
			if err != nil {
				return selection, err
			}
		}
		if selection.selected[guidedCapabilityDocumentDatabase] {
			selection.documentDatabaseManagementUI, err = promptOptionalYesNo(reader, out, "Document database management UI?", false)
			if err != nil {
				return selection, err
			}
		}
		if selection.selected[guidedCapabilityMessagingQueue] || selection.selected[guidedCapabilityMessagingPubSub] || selection.selected[guidedCapabilityMessagingStream] {
			selection.messagingManagementUI, err = promptOptionalYesNo(reader, out, "Messaging management UI?", false)
			if err != nil {
				return selection, err
			}
		}
		if selection.selected[guidedCapabilityObjectStorage] {
			selection.objectStorageManagementUI, err = promptOptionalYesNo(reader, out, "Object storage management UI?", false)
			if err != nil {
				return selection, err
			}
		}
		if selection.selected[guidedCapabilitySecrets] {
			selection.secretsManagementUI, err = promptOptionalYesNo(reader, out, "Secrets management UI?", false)
			if err != nil {
				return selection, err
			}
		}
		if selection.selected[guidedCapabilityIdentity] {
			selection.identityManagementUI, err = promptOptionalYesNo(reader, out, "Identity management UI?", false)
			if err != nil {
				return selection, err
			}
		}
		if selection.selected[guidedCapabilityMetrics] {
			selection.observabilityManagementUI, err = promptOptionalYesNo(reader, out, "Observability management UI (Prometheus)?", false)
			if err != nil {
				return selection, err
			}
		}
	}
	return selection, nil
}

func guidedWorkloadSelection(reader *bufio.Reader, out io.Writer, d appProjectDetection) (string, []string, map[string]string, []repositoryinspect.PortEvidence, error) {
	compose := d.Compose
	workloadServices := append([]string(nil), d.WorkloadServices...)
	ambiguousServices := append([]string(nil), d.AmbiguousServices...)

	if len(d.ComposeCandidates) > 1 {
		var err error
		compose, err = promptCompose(reader, out, d.ComposeCandidates)
		if err != nil {
			return "", nil, nil, nil, err
		}
		analysis, err := repositoryinspect.AnalyzeComposeFile(".", compose)
		if err != nil {
			return "", nil, nil, nil, err
		}
		workloadServices = append([]string(nil), analysis.WorkloadServices...)
		ambiguousServices = append([]string(nil), analysis.AmbiguousServices...)
	}
	if len(ambiguousServices) > 0 {
		confirmedWorkload, err := promptAmbiguousComposeServices(reader, out, ambiguousServices)
		if err != nil {
			return "", nil, nil, nil, err
		}
		workloadServices = uniqueSorted(append(workloadServices, confirmedWorkload...))
	}
	protocols := map[string]string{}
	ports := append([]repositoryinspect.PortEvidence(nil), d.Ports...)
	if strings.TrimSpace(compose) != "" {
		analysis, err := repositoryinspect.AnalyzeComposeFile(".", compose)
		if err != nil {
			return "", nil, nil, nil, err
		}
		for service, protocol := range analysis.WorkloadProtocols {
			protocols[service] = protocol
		}
		ports = append([]repositoryinspect.PortEvidence(nil), analysis.Ports...)
	}
	return compose, workloadServices, protocols, ports, nil
}

func buildGuidedInitManifest(reader *bufio.Reader, out io.Writer, d appProjectDetection, selection guidedInitSelection) (application.Manifest, error) {
	m := detectedApplicationManifest(
		selection.name,
		selection.environment,
		selection.selected[guidedCapabilitySQL],
		selection.selected[guidedCapabilityCache],
		selection.selected[guidedCapabilityObjectStorage],
		selection.selected[guidedCapabilitySecrets],
		selection.compose != "" && len(selection.workloadServices) > 0,
	)
	if len(selection.sqlInstances) > 0 {
		m = application.WithSQLInstances(m, selection.sqlInstances...)
	}
	if len(selection.cacheInstances) > 0 {
		m = application.WithCacheInstances(m, selection.cacheInstances...)
	}
	if selection.selected[guidedCapabilityDurableKeyValue] {
		m.Services.KeyValue = true
		if len(selection.keyValueInstances) > 0 {
			m = application.WithKeyValueInstances(m, selection.keyValueInstances...)
		}
	}
	if selection.selected[guidedCapabilityDocumentDatabase] {
		m.Services.DocumentDatabase = true
		if len(selection.documentDatabaseInstances) > 0 {
			m = application.WithDocumentDatabaseInstances(m, selection.documentDatabaseInstances...)
		}
	}
	if selection.selected[guidedCapabilityMessagingQueue] {
		m.Services.MessagingQueue = true
		if len(selection.messagingQueueInstances) > 0 {
			m = application.WithMessagingQueueInstances(m, selection.messagingQueueInstances...)
		}
	}
	if selection.selected[guidedCapabilityMessagingPubSub] {
		m.Services.MessagingPubSub = true
		if len(selection.messagingPubSubInstances) > 0 {
			m = application.WithMessagingPubSubInstances(m, selection.messagingPubSubInstances...)
		}
	}
	if selection.selected[guidedCapabilityMessagingStream] {
		m.Services.MessagingStream = true
		if len(selection.messagingStreamInstances) > 0 {
			m = application.WithMessagingStreamInstances(m, selection.messagingStreamInstances...)
		}
	}
	if len(selection.objectStorageBuckets) > 0 {
		m = application.WithObjectStorageBuckets(m, selection.objectStorageBuckets...)
	}
	if selection.selected[guidedCapabilityIdentity] {
		m = application.WithIdentity(m)
	}
	m.Services.SQLManagementUI = selection.sqlManagementUI
	m.Services.CacheManagementUI = selection.cacheManagementUI
	m.Services.KeyValueManagementUI = selection.keyValueManagementUI
	m.Services.DocumentDatabaseManagementUI = selection.documentDatabaseManagementUI
	m.Services.MessagingManagementUI = selection.messagingManagementUI
	m.Services.ObjectStorageManagementUI = selection.objectStorageManagementUI
	m.Services.SecretsManagementUI = selection.secretsManagementUI
	m.Services.IdentityManagementUI = selection.identityManagementUI
	m.Services.ObservabilityManagementUI = selection.observabilityManagementUI
	m = applyGuidedSecretPolicies(m, selection.secretPolicies)
	if selection.compose != "" && len(selection.workloadServices) > 0 {
		m = application.WithWorkload(m, filepath.ToSlash(selection.compose), selection.workloadServices...)
		var exposureErr error
		m, exposureErr = addGuidedDetectedExposures(m, selection)
		if exposureErr != nil {
			return application.Manifest{}, exposureErr
		}
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


func addGuidedDetectedExposures(m application.Manifest, selection guidedInitSelection) (application.Manifest, error) {
	selected := map[string]struct{}{}
	for _, service := range selection.workloadServices {
		selected[service] = struct{}{}
	}
	for _, service := range selection.workloadServices {
		protocol := strings.ToLower(strings.TrimSpace(selection.workloadProtocols[service]))
		if protocol == "" {
			continue
		}
		if protocol != "http" && protocol != "https" {
			return application.Manifest{}, fmt.Errorf("workload service %s declares unsupported protocol %q", service, protocol)
		}
		ports := map[int]struct{}{}
		for _, evidence := range selection.workloadPorts {
			if evidence.Service != service {
				continue
			}
			if port, ok := composeTargetPort(evidence.Value); ok {
				ports[port] = struct{}{}
			}
		}
		if len(ports) != 1 {
			return application.Manifest{}, usageError(
				fmt.Sprintf("workload service %s declares %s but its HTTP target port is ambiguous", service, protocol),
				"Declare one unambiguous target port for the service or add exposure.http explicitly.",
			)
		}
		var port int
		for value := range ports {
			port = value
		}
		name := slugifyAppName(service)
		if name == "" {
			return application.Manifest{}, fmt.Errorf("workload service %q cannot be converted to a stable exposure name", service)
		}
		m = application.WithHTTPExposure(m, name, service, port, protocol)
	}
	return m, nil
}

func addGuidedObservability(reader *bufio.Reader, out io.Writer, d appProjectDetection, selection guidedInitSelection, m application.Manifest) (application.Manifest, error) {
	if selection.selected[guidedCapabilityMetrics] {
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
	if selection.selected[guidedCapabilityOTLP] {
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
	if selection.selected[guidedCapabilityLogs] {
		m = application.WithLogsCollection(m, "application")
	}
	return m, nil
}
