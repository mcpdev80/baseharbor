package application

import (
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"path/filepath"
	"strings"
)

func (m Manifest) Validate() error {
	if m.Version != CurrentVersion {
		return fmt.Errorf("unsupported manifest version %d (expected %d)", m.Version, CurrentVersion)
	}
	if m.ApplicationID != "" {
		if err := ValidateApplicationID(m.ApplicationID); err != nil {
			return err
		}
	}
	if err := validateSlug("application name", m.Name); err != nil {
		return err
	}
	if err := validateSlug("environment", m.Environment); err != nil {
		return err
	}
	sql := SQLInstanceNames(m)
	cache := CacheInstanceNames(m)
	keyValue := KeyValueInstanceNames(m)
	objectStorage := ObjectStorageBucketNames(m)
	messagingQueue := MessagingQueueInstanceNames(m)
	messagingPubSub := MessagingPubSubInstanceNames(m)
	messagingStream := MessagingStreamInstanceNames(m)
	if len(sql) == 0 && len(cache) == 0 && len(keyValue) == 0 && len(objectStorage) == 0 && len(messagingQueue) == 0 && len(messagingPubSub) == 0 && len(messagingStream) == 0 && !m.Services.Secrets && !m.Services.Identity && !HasExplicitWorkload(m) && !HasOTLPTelemetry(m) && !HasMetricsSources(m) && !HasLogsCollection(m) {
		return fmt.Errorf("at least one backend service, telemetry binding or explicit Compose workload must be enabled")
	}
	for _, name := range sql {
		if err := validateSlug("SQL instance name", name); err != nil {
			return err
		}
	}
	for _, name := range cache {
		if err := validateSlug("cache instance name", name); err != nil {
			return err
		}
	}
	for _, name := range keyValue {
		if err := validateSlug("key-value database instance name", name); err != nil {
			return err
		}
	}
	cacheNames := make(map[string]struct{}, len(cache))
	for _, name := range cache {
		cacheNames[name] = struct{}{}
	}
	for _, name := range keyValue {
		if _, exists := cacheNames[name]; exists {
			return fmt.Errorf("service instance %q cannot be both cache.key-value and database.key-value", name)
		}
	}
	for _, name := range objectStorage {
		if err := validateSlug("object-storage bucket name", name); err != nil {
			return err
		}
	}
	for _, item := range []struct {
		label string
		names []string
	}{
		{label: "messaging queue instance name", names: messagingQueue},
		{label: "messaging pubsub instance name", names: messagingPubSub},
		{label: "messaging stream instance name", names: messagingStream},
	} {
		for _, name := range item.names {
			if err := validateSlug(item.label, name); err != nil {
				return err
			}
		}
	}
	if err := validateManagementUIPreferences(m.Services); err != nil {
		return err
	}
	if err := validateIdentityRequirements(m); err != nil {
		return err
	}
	if err := validateObservabilityUIPreference(m); err != nil {
		return err
	}
	if (len(m.Secrets.Required) > 0 || len(m.Secrets.Optional) > 0) && !m.Services.Secrets {
		return fmt.Errorf("secret requirements need services.secrets enabled")
	}
	seen := make(map[string]struct{}, len(m.Secrets.Required)+len(m.Secrets.Optional))
	for _, group := range []struct {
		label string
		items []SecretRequirement
	}{
		{label: "required", items: m.Secrets.Required},
		{label: "optional", items: m.Secrets.Optional},
	} {
		for _, requirement := range group.items {
			if err := validateSecretKey(requirement.Name); err != nil {
				return err
			}
			if _, exists := seen[requirement.Name]; exists {
				return fmt.Errorf("duplicate application secret %q", requirement.Name)
			}
			seen[requirement.Name] = struct{}{}
			if err := validateSecretGeneration(requirement.Name, requirement.Generate); err != nil {
				return err
			}
		}
	}
	if err := validateWorkload(m.Workload); err != nil {
		return err
	}
	if err := validateHTTPExposures(m.Workload, m.Exposures); err != nil {
		return err
	}
	if err := validateOTLPTelemetry(m.Workload, m.Telemetry.OTLP); err != nil {
		return err
	}
	if err := validateMetricsSources(m.Workload, m.Metrics.Sources); err != nil {
		return err
	}
	if err := validateLogsRequirements(m.Workload, m.Logs); err != nil {
		return err
	}
	if err := validateRuntimePermissions(m.Workload, m.Runtime.Permissions); err != nil {
		return err
	}
	return nil
}

func validateRuntimePermissions(workload WorkloadConfig, permissions []RuntimePermission) error {
	seenCapabilities := map[string]struct{}{}
	selectedServices := make(map[string]struct{}, len(workload.Services))
	for _, service := range workload.Services {
		selectedServices[service] = struct{}{}
	}
	for _, permission := range permissions {
		spec, err := capability.ParseSpecificationID(capability.SpecificationID(strings.TrimSpace(permission.Capability)))
		if err != nil {
			return fmt.Errorf("runtime permission capability %q: %w", permission.Capability, err)
		}
		canonical := string(spec.ID)
		if _, exists := seenCapabilities[canonical]; exists {
			return fmt.Errorf("duplicate runtime permission capability %q", canonical)
		}
		seenCapabilities[canonical] = struct{}{}
		if len(permission.Services) == 0 {
			return fmt.Errorf("runtime permission %q requires at least one workload service", canonical)
		}
		seenServices := map[string]struct{}{}
		for _, service := range permission.Services {
			if err := validateComposeServiceName(service); err != nil {
				return fmt.Errorf("runtime permission %q: %w", canonical, err)
			}
			if _, exists := seenServices[service]; exists {
				return fmt.Errorf("runtime permission %q repeats workload service %q", canonical, service)
			}
			seenServices[service] = struct{}{}
			if len(selectedServices) > 0 {
				if _, exists := selectedServices[service]; !exists {
					return fmt.Errorf("runtime permission %q targets workload service %q which is not selected", canonical, service)
				}
			}
		}
		if len(permission.Operations) == 0 {
			return fmt.Errorf("runtime permission %q requires at least one operation", canonical)
		}
		seenOperations := map[string]struct{}{}
		for _, operation := range permission.Operations {
			operation = strings.TrimSpace(operation)
			switch operation {
			case "runtime.create", "runtime.get", "runtime.delete", "runtime.rotate":
			default:
				return fmt.Errorf("runtime permission %q has unsupported operation %q", canonical, operation)
			}
			if _, exists := seenOperations[operation]; exists {
				return fmt.Errorf("runtime permission %q repeats operation %q", canonical, operation)
			}
			seenOperations[operation] = struct{}{}
		}
	}
	return nil
}

func validateLogsRequirements(workload WorkloadConfig, logs LogsRequirements) error {
	if len(logs.Collect) == 0 {
		return nil
	}
	if len(workload.Services) == 0 {
		return errors.New("logs collection requires explicit workload.services")
	}
	seen := map[string]struct{}{}
	for _, raw := range logs.Collect {
		source := strings.TrimSpace(strings.ToLower(raw))
		if source != "application" {
			return fmt.Errorf("unsupported logs collect source %q", raw)
		}
		if _, exists := seen[source]; exists {
			return fmt.Errorf("duplicate logs collect source %q", source)
		}
		seen[source] = struct{}{}
	}
	return nil
}

func validateMetricsSources(workload WorkloadConfig, sources []MetricsSourceRequirement) error {
	if len(sources) == 0 {
		return nil
	}
	if len(workload.Services) == 0 {
		return fmt.Errorf("metrics sources require explicit workload.services so service identity is deterministic")
	}
	selected := make(map[string]struct{}, len(workload.Services))
	for _, service := range workload.Services {
		selected[service] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, source := range sources {
		if err := validateSlug("metrics source name", source.Name); err != nil {
			return err
		}
		if _, exists := seen[source.Name]; exists {
			return fmt.Errorf("duplicate metrics source %q", source.Name)
		}
		seen[source.Name] = struct{}{}
		if err := validateComposeServiceName(source.Service); err != nil {
			return fmt.Errorf("metrics source %q: %w", source.Name, err)
		}
		if _, ok := selected[source.Service]; !ok {
			return fmt.Errorf("metrics source %q targets workload service %q which is not selected", source.Name, source.Service)
		}
		if source.Port < 1 || source.Port > 65535 {
			return fmt.Errorf("metrics source %q has invalid target port %d", source.Name, source.Port)
		}
		if source.Path == "" || !strings.HasPrefix(source.Path, "/") || strings.ContainsAny(source.Path, "?#\r\n\x00") {
			return fmt.Errorf("metrics source %q path must be an absolute HTTP path without query or fragment", source.Name)
		}
	}
	return nil
}

func validateOTLPTelemetry(workload WorkloadConfig, requirement *OTLPRequirement) error {
	if requirement == nil {
		return nil
	}
	if len(workload.Services) == 0 {
		return fmt.Errorf("OTLP telemetry requires explicit workload.services so service identity is deterministic")
	}
	if len(requirement.Signals) == 0 {
		return fmt.Errorf("OTLP telemetry requires at least one signal")
	}
	seen := map[string]struct{}{}
	for _, signal := range requirement.Signals {
		signal = strings.TrimSpace(signal)
		switch signal {
		case "traces", "metrics", "logs":
		default:
			return fmt.Errorf("OTLP telemetry signal %q must be traces, metrics or logs", signal)
		}
		if _, exists := seen[signal]; exists {
			return fmt.Errorf("duplicate OTLP telemetry signal %q", signal)
		}
		seen[signal] = struct{}{}
	}
	return nil
}

func validateHTTPExposures(workload WorkloadConfig, exposures []HTTPExposureRequirement) error {
	if len(exposures) > 0 && len(workload.Services) == 0 {
		return fmt.Errorf("managed HTTP exposure requires explicit workload.services so endpoint identity is deterministic")
	}
	seen := make(map[string]struct{}, len(exposures))
	selected := make(map[string]struct{}, len(workload.Services))
	for _, service := range workload.Services {
		selected[service] = struct{}{}
	}
	for _, exposure := range exposures {
		if err := validateSlug("HTTP exposure name", exposure.Name); err != nil {
			return err
		}
		if _, exists := seen[exposure.Name]; exists {
			return fmt.Errorf("duplicate HTTP exposure %q", exposure.Name)
		}
		seen[exposure.Name] = struct{}{}
		if err := validateComposeServiceName(exposure.Service); err != nil {
			return fmt.Errorf("HTTP exposure %q: %w", exposure.Name, err)
		}
		if len(selected) > 0 {
			if _, ok := selected[exposure.Service]; !ok {
				return fmt.Errorf("HTTP exposure %q targets workload service %q which is not selected", exposure.Name, exposure.Service)
			}
		}
		if exposure.Port < 1 || exposure.Port > 65535 {
			return fmt.Errorf("HTTP exposure %q has invalid target port %d", exposure.Name, exposure.Port)
		}
		switch exposure.Protocol {
		case "http", "https":
		default:
			return fmt.Errorf("HTTP exposure %q protocol must be http or https", exposure.Name)
		}
		switch normalizedExposureVisibility(exposure.Visibility) {
		case "public", "internal":
		default:
			return fmt.Errorf("HTTP exposure %q visibility must be public or internal", exposure.Name)
		}
	}
	return nil
}

func normalizedExposureVisibility(value string) string {
	if strings.TrimSpace(value) == "" {
		return "public"
	}
	return strings.TrimSpace(value)
}

func validateSecretGeneration(name string, generation *SecretGeneration) error {
	if generation == nil {
		return nil
	}
	switch generation.Type {
	case "random":
		if generation.Length < 16 || generation.Length > 4096 {
			return fmt.Errorf("generated secret %q random length must be between 16 and 4096", name)
		}
		if generation.Bytes != 0 {
			return fmt.Errorf("generated secret %q random generator must use length, not bytes", name)
		}
	case "hex":
		if generation.Bytes < 16 || generation.Bytes > 1024 {
			return fmt.Errorf("generated secret %q hex bytes must be between 16 and 1024", name)
		}
		if generation.Length != 0 {
			return fmt.Errorf("generated secret %q hex generator must use bytes, not length", name)
		}
	default:
		return fmt.Errorf("generated secret %q uses unsupported generator type %q", name, generation.Type)
	}
	return nil
}

func validateWorkload(workload WorkloadConfig) error {
	if workload.Compose != "" {
		clean := filepath.Clean(workload.Compose)
		if filepath.IsAbs(workload.Compose) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("workload compose path %q must stay inside the application repository", workload.Compose)
		}
		if clean != workload.Compose {
			return fmt.Errorf("workload compose path %q must be normalized", workload.Compose)
		}
	}
	seen := make(map[string]struct{}, len(workload.Services))
	for _, service := range workload.Services {
		if err := validateComposeServiceName(service); err != nil {
			return err
		}
		if _, exists := seen[service]; exists {
			return fmt.Errorf("duplicate workload service %q", service)
		}
		seen[service] = struct{}{}
	}
	return nil
}

func validateComposeServiceName(name string) error {
	if name == "" || len(name) > 128 {
		return fmt.Errorf("invalid workload service name %q", name)
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.') {
			return fmt.Errorf("invalid workload service name %q", name)
		}
	}
	return nil
}

func validateSlug(label, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > 63 {
		return fmt.Errorf("%s must be at most 63 characters", label)
	}
	for i, r := range value {
		valid := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-'
		if !valid {
			return fmt.Errorf("%s %q must contain only lowercase letters, digits and hyphens", label, value)
		}
		if r == '-' && (i == 0 || i == len(value)-1) {
			return fmt.Errorf("%s %q must not start or end with a hyphen", label, value)
		}
	}
	return nil
}

func validateSecretKey(key string) error {
	if key == "" {
		return fmt.Errorf("required secret key is empty")
	}
	if len(key) > 128 {
		return fmt.Errorf("required secret key %q must be at most 128 characters", key)
	}
	if key == "_baseharbor" || strings.HasPrefix(key, "__baseharbor_") {
		return fmt.Errorf("required secret key %q uses a reserved BaseHarbor name", key)
	}
	for i, r := range key {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !valid || (i == 0 && (r == '-' || r == '.')) {
			return fmt.Errorf("invalid required secret key %q", key)
		}
	}
	return nil
}

func validateManagementUIPreferences(services Services) error {
	for _, item := range []struct {
		name    string
		enabled bool
		ui      bool
	}{
		{"sql", services.SQL || len(services.SQLInstances) > 0, services.SQLManagementUI},
		{"cache", services.Cache || len(services.CacheInstances) > 0, services.CacheManagementUI},
		{"key_value", services.KeyValue || len(services.KeyValueInstances) > 0, services.KeyValueManagementUI},
		{"object_storage", services.ObjectStorage || len(services.ObjectStorageBuckets) > 0, services.ObjectStorageManagementUI},
		{"secrets", services.Secrets, services.SecretsManagementUI},
		{"identity", services.Identity, services.IdentityManagementUI},
		{"observability", true, services.ObservabilityManagementUI},
	} {
		if item.ui && !item.enabled {
			return fmt.Errorf("services.%s.management_ui requires the service to be enabled", item.name)
		}
	}
	if services.ObservabilityManagementUI {
		// The concrete observability capability check happens against the full
		// manifest below; this helper only validates service-local dependencies.
	}
	return nil
}

func validateObservabilityUIPreference(m Manifest) error {
	if !m.Services.ObservabilityManagementUI {
		return nil
	}
	if !HasMetricsSources(m) {
		return errors.New("services.observability.management_ui requires managed metrics because Prometheus is the current browser-capable observability surface")
	}
	return nil
}

func validateIdentityRequirements(m Manifest) error {
	if !m.Services.Identity {
		if len(m.Identity.CallbackPaths) != 0 || len(m.Identity.LogoutPaths) != 0 || len(m.Identity.Scopes) != 0 || len(m.Identity.Claims) != 0 ||
			strings.TrimSpace(m.Identity.Authentication.MFA) != "" || len(m.Identity.Authentication.Methods) != 0 || m.Identity.Authentication.Passwordless {
			return errors.New("identity requirements need services.identity enabled")
		}
		return nil
	}
	for label, values := range map[string][]string{
		"callback path": m.Identity.CallbackPaths,
		"logout path":   m.Identity.LogoutPaths,
	} {
		seen := map[string]struct{}{}
		for _, raw := range values {
			value := strings.TrimSpace(raw)
			if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "?#\r\n\x00") {
				return fmt.Errorf("identity %s %q must be an absolute path without scheme, host, query or fragment", label, raw)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("duplicate identity %s %q", label, value)
			}
			seen[value] = struct{}{}
		}
	}
	for label, values := range map[string][]string{"scope": m.Identity.Scopes, "claim": m.Identity.Claims} {
		seen := map[string]struct{}{}
		for _, raw := range values {
			value := strings.TrimSpace(raw)
			if value == "" || strings.ContainsAny(value, " \t\r\n") {
				return fmt.Errorf("identity %s %q is invalid", label, raw)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("duplicate identity %s %q", label, value)
			}
			seen[value] = struct{}{}
		}
	}
	mfa := strings.ToLower(strings.TrimSpace(m.Identity.Authentication.MFA))
	if mfa == "" {
		mfa = "optional"
	}
	if mfa != "optional" && mfa != "required" && mfa != "disabled" {
		return fmt.Errorf("identity authentication mfa must be optional, required or disabled")
	}
	seenMethods := map[string]struct{}{}
	for _, raw := range m.Identity.Authentication.Methods {
		method := strings.ToLower(strings.TrimSpace(raw))
		switch method {
		case "totp", "webauthn", "passkey":
		default:
			return fmt.Errorf("unsupported identity authentication method %q", raw)
		}
		if _, exists := seenMethods[method]; exists {
			return fmt.Errorf("duplicate identity authentication method %q", method)
		}
		seenMethods[method] = struct{}{}
	}
	if mfa == "required" && len(seenMethods) == 0 {
		return errors.New("identity authentication mfa=required needs at least one method")
	}
	if m.Identity.Authentication.Passwordless {
		if _, ok := seenMethods["passkey"]; !ok {
			if _, ok := seenMethods["webauthn"]; !ok {
				return errors.New("identity passwordless authentication requires passkey or webauthn")
			}
		}
	}
	return nil
}
