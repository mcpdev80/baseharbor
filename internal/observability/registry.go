package observability

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type SourceClass string
type SignalKind string

const (
	SourceApplication         SourceClass = "application"
	SourceApplicationProvider SourceClass = "application-provider"
	SourcePlatformProvider    SourceClass = "platform-provider"

	SignalMetrics SignalKind = "metrics"
	SignalLogs    SignalKind = "logs"
	SignalTraces  SignalKind = "traces"
)

type Security struct {
	TLSRequired       bool   `json:"tls_required"`
	Authentication    string `json:"authentication,omitempty"`
	TrustFile         string `json:"trust_file,omitempty"`
	ClientCertificate string `json:"client_certificate,omitempty"`
	ClientKey         string `json:"client_key,omitempty"`
	ServerName        string `json:"server_name,omitempty"`
}

func (s Security) Validate() error {
	switch strings.TrimSpace(s.Authentication) {
	case "", "none", "native", "mtls":
	default:
		return fmt.Errorf("unsupported observability authentication %q", s.Authentication)
	}
	if s.TLSRequired && strings.TrimSpace(s.TrustFile) == "" {
		return errors.New("TLS observability source requires trust material")
	}
	if (s.ClientCertificate == "") != (s.ClientKey == "") {
		return errors.New("observability client certificate and key must be provided together")
	}
	if s.Authentication == "mtls" && (s.ClientCertificate == "" || s.ClientKey == "") {
		return errors.New("mTLS observability source requires client certificate and key")
	}
	return nil
}

type SignalSource struct {
	ID                 string                                   `json:"id"`
	Kind               SignalKind                               `json:"kind"`
	Provider           capability.ProviderKind                  `json:"provider"`
	Class              SourceClass                              `json:"class"`
	Scope              capability.ProviderScope                 `json:"scope"`
	SharingBoundary    string                                   `json:"sharing_boundary,omitempty"`
	OwnerApplication   string                                   `json:"owner_application,omitempty"`
	Network            string                                   `json:"network,omitempty"`
	Target             string                                   `json:"target"`
	Protocol           string                                   `json:"protocol"`
	Path               string                                   `json:"path,omitempty"`
	Mode               capability.ObservabilityRealizationMode `json:"mode,omitempty"`
	SemanticConvention string                                   `json:"semantic_convention,omitempty"`
	Verification       capability.ObservabilityVerificationMode `json:"verification,omitempty"`
	Security           Security                                 `json:"security"`
}

func (s SignalSource) Validate() error {
	if strings.TrimSpace(s.ID) == "" || s.Provider == "" {
		return errors.New("observability signal source requires id and provider")
	}
	switch s.Kind {
	case SignalMetrics, SignalLogs, SignalTraces:
	default:
		return fmt.Errorf("unsupported observability signal kind %q", s.Kind)
	}
	if s.Class != SourceApplication && s.Class != SourceApplicationProvider && s.Class != SourcePlatformProvider {
		return fmt.Errorf("unsupported observability source class %q", s.Class)
	}
	if s.Scope != capability.ScopeShared && s.Scope != capability.ScopeApplication {
		return fmt.Errorf("unsupported observability source scope %q", s.Scope)
	}
	if s.Scope == capability.ScopeApplication && strings.TrimSpace(s.OwnerApplication) == "" {
		return errors.New("application-scoped observability source requires owner application")
	}
	if strings.TrimSpace(s.Target) == "" || strings.TrimSpace(s.Protocol) == "" {
		return errors.New("observability signal source requires target and protocol")
	}
	switch s.Kind {
	case SignalMetrics:
		if strings.TrimSpace(s.Network) == "" || !strings.HasPrefix(strings.TrimSpace(s.Path), "/") {
			return errors.New("observability metrics source requires network and absolute path")
		}
		if s.Protocol != "openmetrics" {
			return fmt.Errorf("observability metrics source requires openmetrics, got %q", s.Protocol)
		}
	case SignalLogs:
		switch s.Protocol {
		case "syslog-rfc5424", "stdout-stderr", "journald", "otlp":
		default:
			return fmt.Errorf("unsupported observability logs protocol %q", s.Protocol)
		}
	case SignalTraces:
		switch s.Protocol {
		case "otlp", "interaction":
		default:
			return fmt.Errorf("unsupported observability traces protocol %q", s.Protocol)
		}
	}
	return s.Security.Validate()
}

// MetricsSource remains the focused compatibility type used by current metrics
// provider call sites. Update converts it into the generic signal registry.
type MetricsSource struct {
	ID               string
	Provider         capability.ProviderKind
	Class            SourceClass
	Scope            capability.ProviderScope
	SharingBoundary  string
	OwnerApplication string
	Network          string
	Target           string
	Path             string
	Scheme           string
	Security         Security
}

func (s MetricsSource) Validate() error {
	return s.signal().Validate()
}

func (s MetricsSource) signal() SignalSource {
	scheme := strings.ToLower(strings.TrimSpace(s.Scheme))
	if scheme == "" {
		scheme = "http"
	}
	security := s.Security
	if scheme == "https" {
		security.TLSRequired = true
	}
	return SignalSource{
		ID: s.ID, Kind: SignalMetrics, Provider: s.Provider, Class: s.Class,
		Scope: s.Scope, SharingBoundary: s.SharingBoundary, OwnerApplication: s.OwnerApplication,
		Network: s.Network, Target: s.Target, Protocol: "openmetrics", Path: s.Path, Security: security,
	}
}

// ProviderSignalRuntime is provider realization state for one declared signal.
// It contains reachability/security details only; portable application intent
// and product-specific collector configuration do not flow through this type.
type ProviderSignalRuntime struct {
	Network  string
	Target   string
	Security Security
}

// ProviderSignalRegistration binds a provider integration descriptor to one
// concrete runtime instance. Multiple signal kinds intentionally share ID so
// Remove(ID) tears down all BaseHarbor-owned observability registrations for
// that provider instance.
type ProviderSignalRegistration struct {
	ID               string
	Descriptor       capability.IntegrationDescriptor
	Class            SourceClass
	Scope            capability.ProviderScope
	SharingBoundary  string
	OwnerApplication string
	Enabled          map[SignalKind]bool
	Signals          map[string]ProviderSignalRuntime
}

func RegisterProviderSignals(registration ProviderSignalRegistration) error {
	if strings.TrimSpace(registration.ID) == "" {
		return errors.New("provider observability registration requires id")
	}
	if err := registration.Descriptor.Validate(); err != nil {
		return fmt.Errorf("provider observability descriptor: %w", err)
	}
	declared := make(map[string]capability.ProviderObservabilitySignal, len(registration.Descriptor.Observability.Signals))
	for _, signal := range registration.Descriptor.Observability.Signals {
		declared[signal.Name] = signal
	}
	for name := range registration.Signals {
		signal, ok := declared[name]
		if !ok {
			return fmt.Errorf("provider %q runtime signal %q is not declared", registration.Descriptor.Provider.Kind, name)
		}
		if !signal.Collectable() {
			return fmt.Errorf("provider %q runtime signal %q is not collectable (status %q)", registration.Descriptor.Provider.Kind, name, signal.Status)
		}
	}

	desired := make([]SignalSource, 0, len(registration.Descriptor.Observability.Signals))
	for _, signal := range registration.Descriptor.Observability.Signals {
		if !signal.Collectable() {
			continue
		}
		kind, err := signalKind(signal.Kind)
		if err != nil {
			return err
		}
		if registration.Enabled != nil && !registration.Enabled[kind] {
			continue
		}
		runtimeSignal, ok := registration.Signals[signal.Name]
		if !ok {
			return fmt.Errorf("provider %q supported signal %q has no runtime realization", registration.Descriptor.Provider.Kind, signal.Name)
		}
		source := SignalSource{
			ID:                 registration.ID,
			Kind:               kind,
			Provider:           registration.Descriptor.Provider.Kind,
			Class:              registration.Class,
			Scope:              registration.Scope,
			SharingBoundary:    registration.SharingBoundary,
			OwnerApplication:   registration.OwnerApplication,
			Network:            runtimeSignal.Network,
			Target:             runtimeSignal.Target,
			Protocol:           signal.Protocol,
			Path:               signal.Path,
			Mode:               signal.Mode,
			SemanticConvention: signal.SemanticConvention,
			Verification:       signal.Verification,
			Security:           runtimeSignal.Security,
		}
		if err := source.Validate(); err != nil {
			return fmt.Errorf("register provider %q signal %q: %w", registration.Descriptor.Provider.Kind, signal.Name, err)
		}
		desired = append(desired, source)
	}

	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		out := sources[:0]
		for _, existing := range sources {
			if existing.ID == registration.ID && existing.Provider == registration.Descriptor.Provider.Kind {
				continue
			}
			out = append(out, existing)
		}
		out = append(out, desired...)
		return out, nil
	})
}

func RuntimeTarget(project, service string) string {
	return "runtime://" + strings.Trim(strings.TrimSpace(project), "/") + "/" + strings.Trim(strings.TrimSpace(service), "/")
}

func ParseRuntimeTarget(target string) (project, service string, ok bool) {
	value := strings.TrimSpace(target)
	if !strings.HasPrefix(value, "runtime://") {
		return "", "", false
	}
	value = strings.TrimPrefix(value, "runtime://")
	project, service, ok = strings.Cut(value, "/")
	if !ok || strings.TrimSpace(project) == "" || strings.TrimSpace(service) == "" || strings.Contains(service, "/") {
		return "", "", false
	}
	return project, service, true
}

func PruneOwnedProviderInstances(provider capability.ProviderKind, class SourceClass, scope capability.ProviderScope, ownerApplication string, keepIDs map[string]struct{}) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		out := sources[:0]
		for _, source := range sources {
			if source.Provider == provider && source.Class == class && source.Scope == scope && source.OwnerApplication == ownerApplication {
				if _, keep := keepIDs[source.ID]; !keep {
					continue
				}
			}
			out = append(out, source)
		}
		return out, nil
	})
}

func signalKind(kind capability.ObservabilitySignalKind) (SignalKind, error) {
	switch kind {
	case capability.ObservabilityMetrics:
		return SignalMetrics, nil
	case capability.ObservabilityLogs:
		return SignalLogs, nil
	case capability.ObservabilityTraces:
		return SignalTraces, nil
	default:
		return "", fmt.Errorf("unsupported provider observability signal kind %q", kind)
	}
}

func registryPath() (string, error) {
	dir, err := bhruntime.DataDir("")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "observability", "signals.json"), nil
}

func Update(source MetricsSource) error {
	return UpdateSignal(source.signal())
}

func UpdateSignal(source SignalSource) error {
	if err := source.Validate(); err != nil {
		return err
	}
	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		out := sources[:0]
		for _, existing := range sources {
			if existing.ID != source.ID || existing.Kind != source.Kind {
				out = append(out, existing)
			}
		}
		out = append(out, source)
		return out, nil
	})
}

func Remove(id string) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		out := sources[:0]
		for _, source := range sources {
			if source.ID != id {
				out = append(out, source)
			}
		}
		return out, nil
	})
}

func List(kind SignalKind, placement capability.ProviderPlacement, applications []string, includeApplicationProviders, includePlatformProviders bool) ([]SignalSource, error) {
	path, err := registryPath()
	if err != nil {
		return nil, err
	}
	sources, err := read(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	allowedApplications := map[string]struct{}{}
	for _, application := range applications {
		if application = strings.TrimSpace(application); application != "" {
			allowedApplications[application] = struct{}{}
		}
	}
	var out []SignalSource
	for _, source := range sources {
		if source.Kind != kind {
			continue
		}
		switch source.Class {
		case SourceApplication:
			continue
		case SourceApplicationProvider:
			if !includeApplicationProviders {
				continue
			}
		case SourcePlatformProvider:
			if !includePlatformProviders {
				continue
			}
		}
		switch placement.Scope {
		case capability.ScopeShared:
			if source.Scope == capability.ScopeShared && strings.TrimSpace(source.SharingBoundary) == strings.TrimSpace(placement.SharingBoundary) {
				out = append(out, source)
			} else if source.Scope == capability.ScopeApplication {
				if _, allowed := allowedApplications[source.OwnerApplication]; allowed {
					out = append(out, source)
				}
			}
		case capability.ScopeApplication:
			if source.Scope == capability.ScopeApplication {
				if _, allowed := allowedApplications[source.OwnerApplication]; allowed {
					out = append(out, source)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func ListLogs(placement capability.ProviderPlacement, applications []string, includeApplicationProviders, includePlatformProviders bool) ([]SignalSource, error) {
	return List(SignalLogs, placement, applications, includeApplicationProviders, includePlatformProviders)
}

func ListTraces(placement capability.ProviderPlacement, applications []string, includeApplicationProviders, includePlatformProviders bool) ([]SignalSource, error) {
	return List(SignalTraces, placement, applications, includeApplicationProviders, includePlatformProviders)
}

func ListMetrics(placement capability.ProviderPlacement, applications []string, includeApplicationProviders, includePlatformProviders bool) ([]MetricsSource, error) {
	signals, err := List(SignalMetrics, placement, applications, includeApplicationProviders, includePlatformProviders)
	if err != nil {
		return nil, err
	}
	out := make([]MetricsSource, 0, len(signals))
	for _, source := range signals {
		scheme := "http"
		if source.Security.TLSRequired {
			scheme = "https"
		}
		out = append(out, MetricsSource{
			ID: source.ID, Provider: source.Provider, Class: source.Class, Scope: source.Scope,
			SharingBoundary: source.SharingBoundary, OwnerApplication: source.OwnerApplication,
			Network: source.Network, Target: source.Target, Path: source.Path, Scheme: scheme, Security: source.Security,
		})
	}
	return out, nil
}

func mutate(path string, fn func([]SignalSource) ([]SignalSource, error)) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	sources, err := read(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sources, err = fn(sources)
	if err != nil {
		return err
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Kind != sources[j].Kind {
			return sources[i].Kind < sources[j].Kind
		}
		return sources[i].ID < sources[j].ID
	})
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func read(path string) ([]SignalSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sources []SignalSource
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, fmt.Errorf("decode observability signal registry: %w", err)
	}
	for _, source := range sources {
		if err := source.Validate(); err != nil {
			return nil, fmt.Errorf("invalid observability signal source %s/%q: %w", source.Kind, source.ID, err)
		}
	}
	return sources, nil
}
