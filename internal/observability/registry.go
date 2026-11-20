package observability

import (
	"bytes"
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
	Scheme             string                                   `json:"scheme,omitempty"`
	Path               string                                   `json:"path,omitempty"`
	Mode               capability.ObservabilityRealizationMode  `json:"mode,omitempty"`
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
		if s.Scheme != "" && s.Scheme != "http" && s.Scheme != "https" {
			return fmt.Errorf("unsupported observability metrics scheme %q", s.Scheme)
		}
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
	Verification     capability.ObservabilityVerificationMode `json:"-"`
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
	if scheme == "https" && s.Class != SourceApplication {
		security.TLSRequired = true
	}
	return SignalSource{
		ID: s.ID, Kind: SignalMetrics, Provider: s.Provider, Class: s.Class,
		Scope: s.Scope, SharingBoundary: s.SharingBoundary, OwnerApplication: s.OwnerApplication,
		Network: s.Network, Target: s.Target, Protocol: "openmetrics", Scheme: scheme, Path: s.Path,
		Verification: s.Verification, Security: security,
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
	// Legacy, when set, makes the registration migrate the pre-scoping source
	// of record INSIDE its own single mutation (card 0428-168): the legacy
	// entry is re-booked onto a derived, project-scoped identity BEFORE the
	// (ID, provider) replacement runs, so the registration can never silently
	// drop a source that predates the ID scoping.
	Legacy *LegacyReconciliation
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
		if legacy := registration.Legacy; legacy != nil {
			sources = RebookLegacySignals(*legacy, sources, registration.ID)
		}
		// Normalization-boundary guard (card 0428-166): the Keycloak signal ID
		// is "keycloak:" + files.Project, which runs the app name through
		// projectToken (lowercase, "." -> "-"); two DISTINCT names normalizing to
		// the same project build the SAME registrationID. The (ID, provider)
		// replacement below would otherwise let the second registration silently
		// overwrite the first owner's source. Fail closed: a registrationID
		// already held by a DIFFERENT owner is rejected (no owner vanishes); the
		// same owner is idempotent.
		for _, existing := range sources {
			if existing.ID == registration.ID && existing.Provider == registration.Descriptor.Provider.Kind &&
				existing.OwnerApplication != "" && existing.OwnerApplication != registration.OwnerApplication {
				return nil, fmt.Errorf("observability registration id %q already owned by %q, not %q: registration rejected (0428-166 normalization collision guard)", registration.ID, existing.OwnerApplication, registration.OwnerApplication)
			}
		}
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

// PruneProviderSignals removes the named signal kinds from exactly one source
// identity without touching its other signals. Registry entries are read-
// modify-written as a whole file, so this is the mechanism a caller uses when
// one signal of one identity is no longer valid while the rest of the
// registration stands.
//
// The predicate carries the full registration key: RegisterProviderSignals
// replaces entries by ID plus provider, so IDs are only unique per provider.
// Matching the provider as well keeps a same-ID registration made by another
// provider, environment or owner application untouched.
func PruneProviderSignals(id string, provider capability.ProviderKind, kinds ...SignalKind) error {
	return pruneSignals(func(source SignalSource) bool {
		return source.ID == id && source.Provider == provider
	}, kinds)
}

func pruneSignals(owned func(SignalSource) bool, kinds []SignalKind) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	drop := map[SignalKind]struct{}{}
	for _, kind := range kinds {
		drop[kind] = struct{}{}
	}
	return mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		out := sources[:0]
		for _, source := range sources {
			if owned(source) {
				if _, unwanted := drop[source.Kind]; unwanted {
					continue
				}
			}
			out = append(out, source)
		}
		return out, nil
	})
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

// ReconcileSignals replaces every signal of one source identity with the
// desired set, in one atomic registry mutation.
//
// Registry IDs are only unique per provider: RegisterProviderSignals keys
// entry replacement on ID plus provider, so the same identity string can
// legitimately exist under two providers (for example a shared collector
// source and an external-OTLP registration). Reconciliation therefore scopes
// removal to the providers the identity is owned by, and a same-ID
// registration under another provider is never touched.
func ReconcileSignals(id string, desired []SignalSource) error {
	owned := map[capability.ProviderKind]struct{}{}
	for i := range desired {
		if desired[i].Provider == "" {
			return fmt.Errorf("observability signal reconciliation source %q has no provider", id)
		}
		owned[desired[i].Provider] = struct{}{}
	}
	return ReconcileProviderSignals(id, owned, desired)
}

// ReconcileProviderSignals is the explicit-ownership form of ReconcileSignals:
// the owned set names every provider whose registrations of this ID the caller
// replaces, independently of the desired set. A caller whose policy disables
// all of an identity's signals passes an empty desired set together with the
// provider it manages - the stale signals of exactly that provider are then
// reclaimed while same-ID registrations of foreign providers stay untouched.
// Each desired signal's provider must be inside the owned set.
func ReconcileProviderSignals(id string, owned map[capability.ProviderKind]struct{}, desired []SignalSource) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("observability signal reconciliation requires id")
	}
	for kind := range owned {
		if kind == "" {
			return fmt.Errorf("observability signal reconciliation id %q names an empty provider", id)
		}
	}
	for i := range desired {
		if desired[i].ID != id {
			return fmt.Errorf("observability signal reconciliation id %q does not match source id %q", id, desired[i].ID)
		}
		if _, ok := owned[desired[i].Provider]; !ok {
			return fmt.Errorf("observability signal reconciliation source %q has a provider outside the owned set", id)
		}
		if err := desired[i].Validate(); err != nil {
			return err
		}
	}
	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		out := sources[:0]
		for _, source := range sources {
			if _, taken := owned[source.Provider]; source.ID == id && taken {
				continue
			}
			out = append(out, source)
		}
		out = append(out, desired...)
		return out, nil
	})
}

// ReconcileMatching replaces exactly the registry entries a caller-owned
// predicate selects with the desired set, in one atomic mutation. It is the
// predicate form of ReconcileProviderSignals for identity spaces that are not
// keyed on ID plus provider: the class-application workload log signals are one
// such space, where the reconciliation boundary is the ID prefix of an
// application/environment, so neither the ID-keyed reconciles nor a provider
// claim (which would delete the metrics twin of the same workload) fit.
// Entries the predicate does not select are never touched, and every desired
// source must satisfy the predicate itself - a caller cannot widen the boundary
// of what it replaces by passing foreign entries alongside its own.
func ReconcileMatching(owned func(SignalSource) bool, desired []SignalSource) error {
	if owned == nil {
		return errors.New("observability signal reconciliation requires a predicate")
	}
	for i := range desired {
		if err := desired[i].Validate(); err != nil {
			return err
		}
		if !owned(desired[i]) {
			return fmt.Errorf("observability signal reconciliation source %q lies outside the owned predicate", desired[i].ID)
		}
	}
	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		out := sources[:0]
		for _, source := range sources {
			if owned(source) {
				continue
			}
			out = append(out, source)
		}
		out = append(out, desired...)
		return out, nil
	})
}

// SignalKindForSignal maps a descriptor signal kind onto the registry's
// SignalKind, the exported twin of the internal registration path.
func SignalKindForSignal(kind capability.ObservabilitySignalKind) (SignalKind, error) {
	return signalKind(kind)
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

func List(kind SignalKind, placement capability.ProviderPlacement, applications []string, includeApplicationSources, includeApplicationProviders, includePlatformProviders bool, requestedEnvironment string) ([]SignalSource, error) {
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
			if !includeApplicationSources {
				continue
			}
		case SourceApplicationProvider:
			if !includeApplicationProviders {
				continue
			}
		case SourcePlatformProvider:
			if !includePlatformProviders {
				continue
			}
		}
		// Central environment filter (card 0428-121, route 2): only an
		// app-scoped caller is scoped to a single environment, so the
		// comparing guard applies there — an app-scoped source is visible only
		// when it resolves to the caller's requested environment. A source whose
		// environment cannot be resolved or resolves elsewhere is excluded
		// (fail-closed) — no cross-environment visibility leak. Shared-platform
		// callers are not environment-scoped: they aggregate the allowed app
		// sources under the shared/platform boundary rule applied downstream.
		if placement.Scope == capability.ScopeApplication && source.Scope == capability.ScopeApplication && !SourceMatchesEnvironment(source, requestedEnvironment) {
			continue
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

func ListLogs(placement capability.ProviderPlacement, applications []string, includeApplicationProviders, includePlatformProviders bool, requestedEnvironment string) ([]SignalSource, error) {
	return List(SignalLogs, placement, applications, false, includeApplicationProviders, includePlatformProviders, requestedEnvironment)
}

func ApplicationMetricsSourceID(application, environment, source string) string {
	return "application:metrics:" + strings.TrimSpace(application) + ":" + strings.TrimSpace(environment) + ":" + strings.TrimSpace(source)
}

// SourceMatchesEnvironment is the single source-driven environment predicate of
// the list surface (card 0428-121, route 2): an app-scoped source is visible
// only when it resolves to an explicit or unambiguously derivable environment
// that equals the caller's requested environment (case-normalised). The
// requested environment is the owning application's manifest environment — the
// capability ProviderPlacement carries none on this base, so no caller passes
// one and the dimension is threaded through the list wrappers explicitly. A
// source whose environment cannot be resolved, or resolves to a different or
// ambiguous environment (for example a hyphenated legacy "prod-west" read as
// "west", or a reserved project token), fails closed: it appears in no single
// environment result instead of leaking into another. Shared/platform sources
// are not environment-scoped here; they keep the shared/platform boundary rule
// applied downstream.
func SourceMatchesEnvironment(source SignalSource, requestedEnvironment string) bool {
	requestedEnvironment = strings.TrimSpace(requestedEnvironment)
	environment, ok := SourceEnvironment(source)
	if !ok {
		return false
	}
	return strings.ToLower(environment) == strings.ToLower(requestedEnvironment)
}

func PruneApplicationMetrics(application, environment string, keepIDs map[string]struct{}) error {
	application = strings.TrimSpace(application)
	environment = strings.TrimSpace(environment)
	if application == "" || environment == "" {
		return errors.New("application metrics pruning requires application and environment")
	}
	prefix := "application:metrics:" + application + ":" + environment + ":"
	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []SignalSource) ([]SignalSource, error) {
		out := sources[:0]
		for _, source := range sources {
			if source.Class == SourceApplication && source.Kind == SignalMetrics && strings.HasPrefix(source.ID, prefix) {
				if _, keep := keepIDs[source.ID]; !keep {
					continue
				}
			}
			out = append(out, source)
		}
		return out, nil
	})
}

func ListApplicationMetrics(placement capability.ProviderPlacement, applications []string, requestedEnvironment string) ([]MetricsSource, error) {
	signals, err := List(SignalMetrics, placement, applications, true, false, false, requestedEnvironment)
	if err != nil {
		return nil, err
	}
	out := make([]MetricsSource, 0, len(signals))
	for _, source := range signals {
		if source.Class != SourceApplication {
			continue
		}
		scheme := source.Scheme
		if scheme == "" {
			scheme = "http"
			if source.Security.TLSRequired {
				scheme = "https"
			}
		}
		out = append(out, MetricsSource{
			ID: source.ID, Provider: source.Provider, Class: source.Class, Scope: source.Scope,
			SharingBoundary: source.SharingBoundary, OwnerApplication: source.OwnerApplication,
			Network: source.Network, Target: source.Target, Path: source.Path,
			Scheme: scheme, Security: source.Security, Verification: source.Verification,
		})
	}
	return out, nil
}

func ListMetrics(placement capability.ProviderPlacement, applications []string, includeApplicationProviders, includePlatformProviders bool, requestedEnvironment string) ([]MetricsSource, error) {
	signals, err := List(SignalMetrics, placement, applications, false, includeApplicationProviders, includePlatformProviders, requestedEnvironment)
	if err != nil {
		return nil, err
	}
	out := make([]MetricsSource, 0, len(signals))
	for _, source := range signals {
		scheme := source.Scheme
		if scheme == "" {
			scheme = "http"
			if source.Security.TLSRequired {
				scheme = "https"
			}
		}
		out = append(out, MetricsSource{
			ID: source.ID, Provider: source.Provider, Class: source.Class, Scope: source.Scope,
			SharingBoundary: source.SharingBoundary, OwnerApplication: source.OwnerApplication,
			Network: source.Network, Target: source.Target, Path: source.Path, Scheme: scheme,
			Security: source.Security, Verification: source.Verification,
		})
	}
	return out, nil
}

func mutate(path string, fn func([]SignalSource) ([]SignalSource, error)) error {
	_, err := mutateChanged(path, fn)
	return err
}

// mutateChanged is the change-aware form of mutate: it takes the SAME state
// lock (the shared "<registry>.lock" flock — no parallel migration lock
// exists), reads the registry, applies the callback, and replaces the registry
// file ONLY when the serialized result actually differs from the bytes on
// disk. A callback that changes nothing — the file already carries exactly
// what the callback produced, or nothing was found and nothing is produced —
// returns changed=false WITHOUT creating, replacing or rewriting the registry
// file. This is the persistence discipline the legacy signal migration relies
// on (card 0428-168): an already-migrated registry stays byte-identical, in
// the style of the precedent no-mutation migration that reports an empty
// changed set so the caller performs no write. mutateChanged never creates a
// file that does not exist.
func mutateChanged(path string, fn func([]SignalSource) ([]SignalSource, error)) (changed bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return false, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	storedBytes, readErr := os.ReadFile(path)
	exists := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return false, readErr
	}
	stored, err := read(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	sources, err := fn(stored)
	if err != nil {
		return false, err
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Kind != sources[j].Kind {
			return sources[i].Kind < sources[j].Kind
		}
		return sources[i].ID < sources[j].ID
	})
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	if exists && bytes.Equal(bytes.TrimSpace(storedBytes), bytes.TrimSpace(data)) {
		return false, nil
	}
	if !exists && string(data) == "[]\n" {
		// Nothing was found and nothing was produced: never create the file.
		return false, nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return true, nil
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
