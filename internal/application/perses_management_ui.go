package application

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// Perses operator-access adapter.
//
// This file owns the one question the Perses surface can never answer from its
// own configuration: which identities may pass through the gateway, and what
// they are allowed to do. Perses authenticates and authorizes its own users,
// but BaseHarbor decides who may reach the surface at all, so a Perses entry in
// public status output is a statement about the current Target and tenant, not
// about a container's settings.
//
// The rules are the ones the management-access contract already states, applied
// to a product that ships with authentication disabled:
//
//   - access is scoped by the actual Target and tenant permissions; a request
//     without a resolved tenant has no basis for a decision, so it fails closed;
//   - unknown ownership, an unverifiable principal, an ambiguous tenant or a
//     missing membership all deny rather than default-allow;
//   - the effective authentication class is whatever is really configured. The
//     adapter never invents a token mode, and an unconfigured Perses surface
//     stays "unsupported" - which also means no datasource-proxy route.

// ErrPersesOwnershipUnknown is the fail-closed verdict when an operator access
// decision cannot attribute the request to one verified identity in one tenant.
var ErrPersesOwnershipUnknown = errors.New("Perses operator access cannot be attributed to a verified identity and tenant")

// PersesAccessDecision is the machine-readable outcome of one operator access
// evaluation. It is what the status surface and any audit record consume; it
// carries no credential material, only references and classes.
type PersesAccessDecision struct {
	Allowed        bool                          `json:"allowed"`
	ReasonCode     string                        `json:"reason_code"`
	Reason         string                        `json:"reason"`
	ActorMode      string                        `json:"actor_mode,omitempty"`
	Tenant         string                        `json:"tenant,omitempty"`
	ManagementRole ManagementRole                `json:"management_role,omitempty"`
	Class          ManagementAuthenticationClass `json:"authentication_class"`
	RoleMappings   []ManagementRoleMapping       `json:"role_mappings,omitempty"`
	// ProxyRoutes lists the route prefixes the gateway refuses for this
	// decision. It is derived, never asserted by the caller.
	ProxyRoutes []string `json:"proxy_routes,omitempty"`
	// ProxyDenyReason explains a refused datasource proxy. Empty when the policy
	// validates.
	ProxyDenyReason string `json:"proxy_deny_reason,omitempty"`
}

// PersesAccessRequest is the input for one evaluation. Target and Environment
// come from the operation context; the tenant and principal are the values the
// authorization boundary already resolved, never self-declared strings.
type PersesAccessRequest struct {
	Target      string
	Environment string
	// Policy is the effective Perses security posture of the surface.
	Policy capability.PersesSecurityPolicy
	// DatasourceEndpoints are the resolved datasource endpoints of the projects
	// this request can reach. The adapter checks them against the policy so a
	// datasource pointing at the Target's own control plane is refused.
	DatasourceEndpoints []string
}

// EvaluatePersesOperatorAccess decides whether one verified operator may use the
// optional Perses management surface, and what the gateway must refuse on their
// behalf.
//
// The order is deliberate: identity and tenancy first, then the release and
// security posture, then the datasource destinations. An unattributable request
// never reaches the point where a policy detail could justify allowing it.
func EvaluatePersesOperatorAccess(ctx context.Context, request PersesAccessRequest) (PersesAccessDecision, error) {
	decision := PersesAccessDecision{
		ReasonCode: "perses_access_denied",
		Reason:     "Perses operator access is not permitted.",
		Class:      ManagementAuthUnsupported,
	}
	principal, authenticated := identity.FromContext(ctx)
	tenant, scoped := tenancy.FromContext(ctx)
	if !authenticated || principal == nil || !principal.IssuerVerified {
		decision.ReasonCode = "operator_authentication_required"
		decision.Reason = "The optional Perses management surface requires a verified operator identity; an anonymous request is never admitted."
		return decision, ErrPersesOwnershipUnknown
	}
	if !scoped || tenant == nil || strings.TrimSpace(tenant.TenantID) == "" || strings.TrimSpace(tenant.ExternalIdentityID) == "" {
		decision.ReasonCode = "tenant_membership_unresolved"
		decision.Reason = "The verified operator carries no single resolved tenant membership, so Perses access cannot be scoped to a Target."
		return decision, ErrPersesOwnershipUnknown
	}
	if strings.TrimSpace(request.Target) == "" {
		decision.ReasonCode = "target_unattributed"
		decision.Reason = "Perses operator access requires the Target the surface belongs to; an unattributed request fails closed."
		return decision, ErrPersesOwnershipUnknown
	}
	role, hasRole := persesManagementRoleFrom(tenant.Roles)
	if !hasRole {
		decision.ReasonCode = "tenant_permission_denied"
		decision.Reason = "The resolved tenant membership carries no BaseHarbor management role, so the Perses surface is not reachable."
		return decision, persesDenied(decision)
	}
	decision.ActorMode = "authenticated"
	decision.Tenant = tenant.TenantID
	decision.ManagementRole = role
	decision.Class, decision.RoleMappings = persesEffectiveAccess(request.Policy)

	denied, reason := serviceaccess.PersesDeniedProxyPaths(request.Policy)
	decision.ProxyRoutes = denied
	decision.ProxyDenyReason = reason
	if err := capability.ValidatePersesSecurityPolicy(request.Policy); err != nil {
		if errors.Is(err, &capability.PersesAnonymousExposureError{}) {
			decision.ReasonCode = "perses_anonymous_exposure"
		} else {
			decision.ReasonCode = "perses_release_unsupported"
		}
		decision.Reason = err.Error()
		return decision, err
	}
	for _, endpoint := range request.DatasourceEndpoints {
		verdict := capability.PersesProxyDestinationAllowed(request.Policy, endpoint)
		if verdict.Allowed || verdict.VerifiedAtConnection {
			continue
		}
		if len(verdict.Denied) > 0 {
			decision.ReasonCode = "perses_datasource_denied"
			decision.Reason = fmt.Sprintf("Perses datasource %q is refused: %s", endpoint, verdict.Denied[0].Reason)
			return decision, errors.New(decision.Reason)
		}
		decision.ReasonCode = "perses_datasource_denied"
		decision.Reason = fmt.Sprintf("Perses datasource %q is not reachable under the current proxy policy.", endpoint)
		return decision, errors.New(decision.Reason)
	}
	if !persesRoleAllows(role, request.Policy) {
		decision.ReasonCode = "perses_role_denied"
		decision.Reason = fmt.Sprintf("The management role %q may not use the Perses management surface under the current policy.", role)
		return decision, persesDenied(decision)
	}
	decision.Allowed = true
	decision.ReasonCode = "perses_access_allowed"
	decision.Reason = "The verified operator holds a management role that permits the Perses management surface."
	return decision, nil
}

func persesDenied(decision PersesAccessDecision) error {
	return fmt.Errorf("%s: %s", decision.ReasonCode, decision.Reason)
}

// persesEffectiveAccess maps the configured Perses posture onto the shared
// management-access vocabulary. It reports only what the release actually
// provides: no provider configured is "unsupported", native accounts are a
// provider-native credential, and an OIDC or OAuth provider is the
// standards-based class. BaseHarbor has no supported proprietary token mode for
// this surface, so it is never claimed.
func persesEffectiveAccess(policy capability.PersesSecurityPolicy) (ManagementAuthenticationClass, []ManagementRoleMapping) {
	if !policy.AuthenticationEnabled {
		return ManagementAuthUnsupported, PersesUnsupportedRoleMappings()
	}
	switch class := ManagementAuthenticationClass(policy.Release.AuthenticationClass); class {
	case ManagementAuthNativeOIDC, ManagementAuthStandardsAdapter, ManagementAuthNativeCredential:
		return class, PersesRoleMappingsFor(class)
	default:
		return ManagementAuthUnsupported, PersesUnsupportedRoleMappings()
	}
}

// PersesRoleMappingsFor returns the management-role mapping declared for one
// authentication class, classified exact, limited or unsupported. BaseHarbor
// must not claim finer authorization than Perses can enforce, so a mode without
// a native enforcement point says so instead of inventing one.
func PersesRoleMappingsFor(class ManagementAuthenticationClass) []ManagementRoleMapping {
	switch class {
	case ManagementAuthNativeOIDC, ManagementAuthStandardsAdapter:
		return PersesOIDCRoleMappings()
	case ManagementAuthNativeCredential:
		return PersesNativeRoleMappings()
	default:
		return PersesUnsupportedRoleMappings()
	}
}

// PersesOIDCRoleMappings maps the four provider-neutral management roles onto
// Perses roles when an OIDC or standards-based provider enforces them through
// claim-based role assignment. Perses authorizes with roles and role bindings,
// so administrative, editing and viewing access are enforceable. Auditing has no
// dedicated native role, so it is limited to what the read-only role can see.
func PersesOIDCRoleMappings() []ManagementRoleMapping {
	return []ManagementRoleMapping{
		{Role: ManagementRoleAdmin, Level: ManagementRoleMappingExact, Native: "admin"},
		{Role: ManagementRoleEdit, Level: ManagementRoleMappingExact, Native: "edit"},
		{Role: ManagementRoleView, Level: ManagementRoleMappingExact, Native: "view"},
		{Role: ManagementRoleAudit, Level: ManagementRoleMappingLimited, Native: "view"},
	}
}

// PersesNativeRoleMappings is the mapping when Perses authenticates with its own
// native users. Administrative access is exact; editing and viewing depend on
// the project role bindings BaseHarbor provisions, so they are limited; the
// audit role has no native enforcement point and is reported unsupported.
func PersesNativeRoleMappings() []ManagementRoleMapping {
	return []ManagementRoleMapping{
		{Role: ManagementRoleAdmin, Level: ManagementRoleMappingExact, Native: "admin"},
		{Role: ManagementRoleEdit, Level: ManagementRoleMappingLimited, Native: "edit"},
		{Role: ManagementRoleView, Level: ManagementRoleMappingLimited, Native: "view"},
		{Role: ManagementRoleAudit, Level: ManagementRoleMappingUnsupported},
	}
}

// PersesUnsupportedRoleMappings states that no management role is enforceable on
// an unauthenticated surface. Everything is unsupported, including read access:
// the surface is not usable, not merely unprotected.
func PersesUnsupportedRoleMappings() []ManagementRoleMapping {
	return []ManagementRoleMapping{
		{Role: ManagementRoleAdmin, Level: ManagementRoleMappingUnsupported},
		{Role: ManagementRoleEdit, Level: ManagementRoleMappingUnsupported},
		{Role: ManagementRoleView, Level: ManagementRoleMappingUnsupported},
		{Role: ManagementRoleAudit, Level: ManagementRoleMappingUnsupported},
	}
}

// persesManagementRoleFrom maps the resolved tenant roles onto the single
// provider-neutral management role with the widest reach, the same vocabulary
// management-access-v1 documents. Unknown role names contribute nothing, so a
// membership that carries only application or unrecognized roles has no
// management access at all and fails closed.
func persesManagementRoleFrom(roles []string) (ManagementRole, bool) {
	const (
		rankView  = 1
		rankEdit  = 2
		rankAdmin = 3
	)
	ranks := map[string]int{
		"admin":  rankAdmin,
		"owner":  rankAdmin,
		"editor": rankEdit,
		"edit":   rankEdit,
		"update": rankEdit,
		"viewer": rankView,
		"view":   rankView,
		"read":   rankView,
		"reader": rankView,
		"audit":  rankView,
	}
	best := ManagementRole("")
	bestRank := 0
	for _, raw := range roles {
		rank, ok := ranks[strings.ToLower(strings.TrimSpace(raw))]
		if !ok || rank < bestRank {
			continue
		}
		switch rank {
		case rankAdmin:
			best = ManagementRoleAdmin
		case rankEdit:
			best = ManagementRoleEdit
		default:
			best = ManagementRoleView
		}
		bestRank = rank
	}
	return best, bestRank > 0
}

// persesRoleAllows reports whether a management role can be honored by the
// surface under this policy. A read-only deployment cannot serve a mutating
// role, and advertising access the surface cannot honor would be a false claim,
// so that combination denies.
func persesRoleAllows(role ManagementRole, policy capability.PersesSecurityPolicy) bool {
	if role == "" {
		return false
	}
	if policy.ReadOnlyMode && (role == ManagementRoleAdmin || role == ManagementRoleEdit) {
		return false
	}
	return true
}

// PersesGatewayPlanFor derives the protected gateway plan for one selected
// Perses surface from the existing transport and authorization contracts. It
// returns the fail-closed error when the surface cannot be secured.
func PersesGatewayPlanFor(m Manifest, policy capability.PersesSecurityPolicy) (serviceaccess.PersesGatewayPlan, error) {
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return serviceaccess.PersesGatewayPlan{}, errors.New("the Perses management surface is not selected")
	}
	authentication, err := serviceaccess.Resolve(m.Environment, serviceaccess.PersesGatewayProvider, serviceaccess.AuthenticationMTLS)
	if err != nil {
		return serviceaccess.PersesGatewayPlan{}, err
	}
	denied, err := serviceaccess.PersesProxySSRFDenyNetworks(serviceaccess.PersesGatewayRequirements{
		OperatorNetworks:         PersesTargetDenyNetworks(),
		KubernetesServiceAddress: strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST")),
	})
	if err != nil {
		return serviceaccess.PersesGatewayPlan{}, err
	}
	effective := policy
	effective.DeniedNetworks = append(append([]string(nil), policy.DeniedNetworks...), denied...)
	// Shared-namespace topology (the production form): Perses binds the
	// loopback of the namespace its gateway joins, and the gateway routes to
	// the backend-network alias of the Perses service. The rendered service
	// definition gets no published port and no 0.0.0.0 bind. The
	// BindAddress policy field belongs to the configuration renderer (it
	// states what the process binds); the gateway's loopback bind is
	// command-line-pinned and never routes through the manifest alias here.
	upstreamHost := serviceaccess.PersesServiceName
	if raw := strings.TrimSpace(m.Name); raw != "" {
		upstreamHost = devaccess.ApplicationAlias(m.Name, serviceaccess.PersesServiceName)
	}
	requirements := serviceaccess.PersesGatewayRequirements{
		Upstream:       "http://" + upstreamHost + ":" + strconv.Itoa(serviceaccess.PersesUpstreamPort),
		Policy:         effective,
		Authentication: authentication.Authentication,
		AuthTokenFile:  authentication.AuthTokenFile,
		ServerNames:    []string{upstreamHost, serviceaccess.PersesServiceName},
	}
	// The effective policy carries how the surface is reached (the backend
	// alias) so the rendered configuration and the persisted plan state it
	// once, deterministically.
	requirements.Policy.ServiceAddress = upstreamHost
	plan, err := serviceaccess.BuildPersesGateway(requirements)
	if err != nil {
		return serviceaccess.PersesGatewayPlan{}, err
	}
	plan.Spec.Upstreams = nil
	return plan, nil
}

// PersesTargetDenyNetworks returns the site-local ranges a Perses instance on a
// Target must never proxy into. Upstream allows private networks by default, so
// this deny list is what turns that permissive default into a tenant-scoped
// boundary rather than a cluster-wide one.
func PersesTargetDenyNetworks() []string {
	out := []string{"10.0.0.0/8", "100.64.0.0/10", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}
	sort.Strings(out)
	return out
}

// ValidatePersesEndpointTrust checks that a Perses access endpoint is served
// over HTTPS and does not publish a routable literal address. It exists so a URL
// rendered into status output can be tested for transport trust instead of
// assumed. An empty endpoint is trusted-by-absence: the surface reports no URL
// until it is actually reachable.
func ValidatePersesEndpointTrust(rawURL string) error {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("parse Perses management endpoint: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("Perses management endpoint %q must be served over HTTPS", value)
	}
	host := strings.TrimSuffix(strings.TrimPrefix(parsed.Hostname(), "["), "]")
	if host == "" {
		return fmt.Errorf("Perses management endpoint %q has no host", value)
	}
	if addr, err := netip.ParseAddr(host); err == nil && !addr.Unmap().IsLoopback() {
		return fmt.Errorf("Perses management endpoint %q publishes a routable literal address; publish the gateway on a named TLS authority instead", value)
	}
	return nil
}
