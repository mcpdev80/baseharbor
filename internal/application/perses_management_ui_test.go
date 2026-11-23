package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func persesVerifiedContext(roles ...string) context.Context {
	// A no-argument call must never produce a tenant with empty roles: that
	// fixture would assert the wrong thing about authorization.
	if len(roles) == 0 {
		panic("the verified-identity fixture needs at least one role")
	}
	principal := &identity.Principal{
		Issuer:         "https://idp.baseharbor.test/realms/baseharbor",
		Subject:        "b7a4e0f1-0d2f-4a5c-9e31-1c2d3e4f5061",
		Assurance:      "oidc-verified",
		Methods:        []string{"oidc"},
		Groups:         roles,
		IssuerVerified: true,
	}
	ctx := identity.WithPrincipal(context.Background(), principal)
	memberships := make([]tenancy.Membership, 0, len(roles))
	for _, role := range roles {
		memberships = append(memberships, tenancy.Membership{TenantID: "tenant-alpha", Role: role})
	}
	scoped, err := tenancy.Resolve(principal.Subject, memberships)
	if err != nil || scoped == nil {
		// The helper is only used with concrete roles; an empty role list would
		// resolve to a tenant with no roles, which is a valid negative fixture.
		if scoped == nil {
			panic("test fixture must resolve a tenant: " + tenancyErrorText(err))
		}
	}
	return tenancy.WithContext(ctx, scoped)
}

func tenancyErrorText(err error) string {
	if err == nil {
		return "no error"
	}
	return err.Error()
}

func persesVerifiedRequest(roles ...string) (context.Context, PersesAccessRequest) {
	return persesVerifiedContext(roles...), PersesAccessRequest{
		Target:      "lab-01",
		Environment: "production",
		Policy:      capability.DefaultPersesSecurityPolicy(),
	}
}

func TestEvaluatePersesOperatorAccessFailsClosedWithoutIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
		req  PersesAccessRequest
		want string
	}{
		{
			name: "anonymous request",
			ctx:  context.Background(),
			req:  PersesAccessRequest{Target: "lab-01", Environment: "production", Policy: capability.DefaultPersesSecurityPolicy()},
			want: "operator_authentication_required",
		},
		{
			name: "unverified principal",
			ctx:  identity.WithPrincipal(context.Background(), &identity.Principal{Issuer: "https://idp.test", Subject: "someone", IssuerVerified: false}),
			req:  PersesAccessRequest{Target: "lab-01", Environment: "production", Policy: capability.DefaultPersesSecurityPolicy()},
			want: "operator_authentication_required",
		},
		{
			name: "verified identity without tenant membership",
			ctx:  identity.WithPrincipal(context.Background(), &identity.Principal{Issuer: "https://idp.test", Subject: "someone", IssuerVerified: true}),
			req:  PersesAccessRequest{Target: "lab-01", Environment: "production", Policy: capability.DefaultPersesSecurityPolicy()},
			want: "tenant_membership_unresolved",
		},
		{
			name: "membership without a management role",
			ctx:  persesVerifiedContext("application-owner"),
			req:  PersesAccessRequest{Target: "lab-01", Environment: "production", Policy: capability.DefaultPersesSecurityPolicy()},
			want: "tenant_permission_denied",
		},
		{
			name: "attributed identity on an unknown target",
			ctx:  persesVerifiedContext("admin"),
			req:  PersesAccessRequest{Target: "", Environment: "production", Policy: capability.DefaultPersesSecurityPolicy()},
			want: "target_unattributed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := EvaluatePersesOperatorAccess(tc.ctx, tc.req)
			if err == nil {
				t.Fatal("an unattributable or unauthorized Perses request must fail closed")
			}
			if decision.Allowed {
				t.Fatalf("the decision must not be allowed: %+v", decision)
			}
			if decision.ReasonCode != tc.want {
				t.Fatalf("reason code = %q, want %q (%v)", decision.ReasonCode, tc.want, err)
			}
			if decision.Class != ManagementAuthUnsupported {
				t.Fatalf("a denied decision must never claim an authentication class, got %q", decision.Class)
			}
		})
	}
}

// TestEvaluatePersesAnonymousDenialNamesNextCommand binds the actionability of
// the anonymous Perses denial to its wording: the operator must learn which
// precondition is missing and which command to run next, without reading source.
func TestEvaluatePersesAnonymousDenialNamesNextCommand(t *testing.T) {
	decision, err := EvaluatePersesOperatorAccess(
		context.Background(),
		PersesAccessRequest{Target: "lab-01", Environment: "production", Policy: capability.DefaultPersesSecurityPolicy()},
	)
	if err == nil {
		t.Fatal("an anonymous Perses request must fail closed")
	}
	if !errors.Is(err, ErrPersesOwnershipUnknown) {
		t.Fatalf("anonymous request must return the ownership sentinel, got %v", err)
	}
	if decision.ReasonCode != "operator_authentication_required" {
		t.Fatalf("reason code = %q, want operator_authentication_required", decision.ReasonCode)
	}
	if !strings.Contains(decision.Reason, "baha login") {
		t.Fatalf("the denial must name the next command 'baha login', got %q", decision.Reason)
	}
	if !strings.Contains(decision.Reason, "verified operator identity") {
		t.Fatalf("the denial must name the missing precondition, got %q", decision.Reason)
	}
	if !strings.Contains(decision.Reason, "for the selected Target/environment") {
		t.Fatalf("the denial must name the path, got %q", decision.Reason)
	}
}

func TestEvaluatePersesOperatorAccessAllowsVerifiedRoles(t *testing.T) {
	for _, tc := range []struct {
		role string
		want ManagementRole
	}{
		{role: "admin", want: ManagementRoleAdmin},
		{role: "editor", want: ManagementRoleEdit},
		{role: "viewer", want: ManagementRoleView},
		{role: "audit", want: ManagementRoleView},
	} {
		t.Run(tc.role, func(t *testing.T) {
			ctx, request := persesVerifiedRequest(tc.role)
			decision, err := EvaluatePersesOperatorAccess(ctx, request)
			if err != nil {
				t.Fatalf("a verified operator with the %q role must be admitted: %v", tc.role, err)
			}
			if !decision.Allowed {
				t.Fatalf("decision must allow %q: %+v", tc.role, decision)
			}
			if decision.ManagementRole != tc.want {
				t.Fatalf("role %q must map to the %q management role, got %q", tc.role, tc.want, decision.ManagementRole)
			}
			if decision.Tenant != "tenant-alpha" {
				t.Fatalf("access must be scoped to the resolved tenant, got %q", decision.Tenant)
			}
			if len(decision.ProxyRoutes) == 0 || decision.ProxyRoutes[0] != capability.PersesProxyPrefix {
				t.Fatalf("the datasource proxy must stay refused even for an allowed operator, got %v", decision.ProxyRoutes)
			}
			if err := ValidateManagementRoleMappings(decision.RoleMappings); err != nil {
				t.Fatalf("the declared role mappings must be valid: %v", err)
			}
		})
	}
}

func TestEvaluatePersesOperatorAccessRefusesAnonymousSurfaceEvenForAdmin(t *testing.T) {
	ctx, request := persesVerifiedRequest("admin")
	request.Policy = capability.AnonymousPersesSecurityPolicy()
	decision, err := EvaluatePersesOperatorAccess(ctx, request)
	if err == nil || decision.Allowed {
		t.Fatalf("an anonymous Perses surface must be refused even for a verified admin: %+v %v", decision, err)
	}
	if decision.ReasonCode != "perses_anonymous_exposure" {
		t.Fatalf("reason code = %q, want perses_anonymous_exposure", decision.ReasonCode)
	}
	if decision.Class != ManagementAuthUnsupported {
		t.Fatalf("an anonymous surface must report %q, got %q", ManagementAuthUnsupported, decision.Class)
	}
	if decision.ProxyDenyReason == "" {
		t.Fatal("the refusal must explain why the datasource proxy is denied")
	}
	for _, mapping := range decision.RoleMappings {
		if mapping.Level != ManagementRoleMappingUnsupported {
			t.Fatalf("no management role is enforceable on an anonymous surface, got %+v", mapping)
		}
	}
}

func TestEvaluatePersesOperatorAccessRefusesVulnerablePin(t *testing.T) {
	ctx, request := persesVerifiedRequest("admin")
	request.Policy = capability.DefaultPersesSecurityPolicy()
	request.Policy.Release.Version = "v0.54.0"
	decision, err := EvaluatePersesOperatorAccess(ctx, request)
	if err == nil || decision.Allowed {
		t.Fatalf("a Perses pin without the SSRF fixes must be refused: %+v %v", decision, err)
	}
	if decision.ReasonCode != "perses_release_unsupported" {
		t.Fatalf("reason code = %q, want perses_release_unsupported", decision.ReasonCode)
	}
}

func TestEvaluatePersesOperatorAccessRefusesSSRFDatasourceEndpoint(t *testing.T) {
	ctx, request := persesVerifiedRequest("admin")
	for _, tc := range []struct {
		name     string
		endpoint string
		want     string
	}{
		{name: "target control plane", endpoint: "http://127.0.0.1:9090", want: "default deny set"},
		{name: "cloud metadata", endpoint: "http://169.254.169.254/latest/meta-data/iam/security-credentials/", want: "cloud metadata"},
		{name: "credential bearing URL", endpoint: "https://admin:hunter2@prom.internal", want: "credentials"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scoped := request
			scoped.Policy.AllowedSchemes = []string{"http", "https"}
			scoped.DatasourceEndpoints = []string{tc.endpoint}
			decision, err := EvaluatePersesOperatorAccess(ctx, scoped)
			if err == nil || decision.Allowed {
				t.Fatalf("the datasource %q must refuse access: %+v %v", tc.endpoint, decision, err)
			}
			if decision.ReasonCode != "perses_datasource_denied" {
				t.Fatalf("reason code = %q, want perses_datasource_denied", decision.ReasonCode)
			}
			if !strings.Contains(decision.Reason, tc.want) {
				t.Fatalf("reason %q must mention %q", decision.Reason, tc.want)
			}
			// The refusal must still name the refused proxy route: an operator who
			// cannot reach the surface must not gain an unproxied path either.
			if len(decision.ProxyRoutes) == 0 {
				t.Fatal("a datasource refusal must keep the proxy route denied")
			}
		})
	}
}

func TestEvaluatePersesOperatorAccessAllowsScopedDatasource(t *testing.T) {
	ctx, request := persesVerifiedRequest("editor")
	request.DatasourceEndpoints = []string{"https://prometheus.monitoring.svc:9090"}
	decision, err := EvaluatePersesOperatorAccess(ctx, request)
	if err != nil || !decision.Allowed {
		t.Fatalf("a named https datasource must be admitted under the policy: %+v %v", decision, err)
	}
	if decision.ManagementRole != ManagementRoleEdit {
		t.Fatalf("role mapping = %q, want edit", decision.ManagementRole)
	}
}

func TestEvaluatePersesOperatorAccessReadOnlyDeniesMutatingRoles(t *testing.T) {
	ctx, _ := persesVerifiedRequest("admin")
	request := PersesAccessRequest{Target: "lab-01", Environment: "production", Policy: capability.DefaultPersesSecurityPolicy()}
	request.Policy.ReadOnlyMode = true
	decision, err := EvaluatePersesOperatorAccess(ctx, request)
	if err == nil || decision.Allowed {
		t.Fatalf("a read-only Perses deployment cannot honor an admin mutation role and must say so: %+v %v", decision, err)
	}
	if decision.ReasonCode != "perses_role_denied" {
		t.Fatalf("reason code = %q, want perses_role_denied", decision.ReasonCode)
	}
	viewCtx, viewRequest := persesVerifiedRequest("viewer")
	viewRequest.Policy.ReadOnlyMode = true
	if viewDecision, viewErr := EvaluatePersesOperatorAccess(viewCtx, viewRequest); viewErr != nil || !viewDecision.Allowed {
		t.Fatalf("a read-only deployment must still admit the view role: %+v %v", viewDecision, viewErr)
	}
}

func TestPersesRoleMappingsNeverClaimUnsupportedAuthorization(t *testing.T) {
	for _, tc := range []struct {
		class ManagementAuthenticationClass
		want  map[ManagementRole]ManagementRoleMappingLevel
	}{
		{
			class: ManagementAuthNativeOIDC,
			want: map[ManagementRole]ManagementRoleMappingLevel{
				ManagementRoleAdmin: ManagementRoleMappingExact,
				ManagementRoleEdit:  ManagementRoleMappingExact,
				ManagementRoleView:  ManagementRoleMappingExact,
				// Perses has no dedicated audit role: claiming exact here would be a
				// finer authorization claim than the provider can enforce.
				ManagementRoleAudit: ManagementRoleMappingLimited,
			},
		},
		{
			class: ManagementAuthNativeCredential,
			want: map[ManagementRole]ManagementRoleMappingLevel{
				ManagementRoleAdmin: ManagementRoleMappingExact,
				ManagementRoleEdit:  ManagementRoleMappingLimited,
				ManagementRoleView:  ManagementRoleMappingLimited,
				ManagementRoleAudit: ManagementRoleMappingUnsupported,
			},
		},
		{
			class: ManagementAuthUnsupported,
			want: map[ManagementRole]ManagementRoleMappingLevel{
				ManagementRoleAdmin: ManagementRoleMappingUnsupported,
				ManagementRoleEdit:  ManagementRoleMappingUnsupported,
				ManagementRoleView:  ManagementRoleMappingUnsupported,
				ManagementRoleAudit: ManagementRoleMappingUnsupported,
			},
		},
	} {
		t.Run(string(tc.class), func(t *testing.T) {
			mappings := PersesRoleMappingsFor(tc.class)
			if err := ValidateManagementRoleMappings(mappings); err != nil {
				t.Fatalf("role mappings must satisfy the shared contract: %v", err)
			}
			if len(mappings) != len(tc.want) {
				t.Fatalf("every management role must be classified, got %d mappings", len(mappings))
			}
			for _, mapping := range mappings {
				if got, ok := tc.want[mapping.Role]; !ok || got != mapping.Level {
					t.Fatalf("role %q mapped %q, want %q for class %q", mapping.Role, mapping.Level, got, tc.class)
				}
				if mapping.Level == ManagementRoleMappingUnsupported && mapping.Native != "" {
					t.Fatalf("an unsupported mapping must name no native role, got %+v", mapping)
				}
			}
			// An unknown class must degrade to unsupported, never to a permissive set.
			if unknown := PersesRoleMappingsFor("grafana-admin"); len(unknown) != 4 {
				t.Fatalf("an unknown class must report all four roles unsupported, got %+v", unknown)
			}
		})
	}
}

func TestPersesAccessDecisionCarriesNoSecretMaterial(t *testing.T) {
	ctx, request := persesVerifiedRequest("admin")
	request.Policy.Release.AuthenticationClass = string(ManagementAuthNativeCredential)
	request.DatasourceEndpoints = []string{"https://prometheus.internal:9090"}
	decision, err := EvaluatePersesOperatorAccess(ctx, request)
	if err != nil {
		t.Fatalf("a verified operator on a protected surface must be admitted: %v", err)
	}
	if decision.Class != ManagementAuthNativeCredential {
		t.Fatalf("the effective class must be reported, got %q", decision.Class)
	}
	data, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	rendered := string(data)
	for _, forbidden := range []string{"password", "token", "secret", "hunter2", "encryption_key", "private"} {
		if strings.Contains(strings.ToLower(rendered), forbidden) {
			t.Fatalf("an audit-safe decision must not mention %q, got %s", forbidden, rendered)
		}
	}
	if !strings.Contains(rendered, `"reason_code"`) || !strings.Contains(rendered, `"authentication_class"`) {
		t.Fatalf("the decision must expose its machine-readable verdict, got %s", rendered)
	}
}

func TestPersesGatewayPlanForRequiresSelectionAndPolicy(t *testing.T) {
	manifest := Manifest{Environment: "production"}
	if _, err := PersesGatewayPlanFor(manifest, capability.DefaultPersesSecurityPolicy()); err == nil {
		t.Fatal("an unselected Perses surface must not produce a gateway plan")
	}
	manifest.Services.ObservabilityManagementUI = true
	manifest.Services.Perses = true
	if _, err := PersesGatewayPlanFor(manifest, capability.PersesSecurityPolicy{}); err == nil {
		t.Fatal("a zero-value posture (no pin, no authentication) must not produce a gateway plan")
	}
	if _, err := PersesGatewayPlanFor(manifest, capability.AnonymousPersesSecurityPolicy()); err == nil {
		t.Fatal("the gateway plan must re-check the posture rather than trust its caller")
	}
	policy := capability.DefaultPersesSecurityPolicy()
	policy.EncryptionKeyDeclared = true
	plan, err := PersesGatewayPlanFor(manifest, policy)
	if err != nil {
		t.Fatalf("a selected, protected Perses surface must produce a gateway plan: %v", err)
	}
	if plan.Spec.ServiceName != "perses-ui-access" {
		t.Fatalf("the plan must name the gateway service that owns operator access, got %q", plan.Spec.ServiceName)
	}
	if len(plan.DeniedPaths) == 0 || plan.DeniedPaths[0] != capability.PersesProxyPrefix {
		t.Fatalf("the derived plan must refuse the datasource proxy, got %v", plan.DeniedPaths)
	}
	joined := strings.Join(plan.Policy.DeniedNetworks, " ")
	for _, want := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the Target deny list must include %s, got %v", want, plan.Policy.DeniedNetworks)
		}
	}
}

func TestValidatePersesEndpointTrustRejectsPlainOrRoutableEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
		want string
	}{
		{name: "plain http", url: "http://127.0.0.1:3000", want: "HTTPS"},
		{name: "routable literal", url: "https://10.1.2.3:8443", want: "routable literal"},
		{name: "no host", url: "https://", want: "no host"},
		{name: "unparseable", url: "https://%zz", want: "parse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePersesEndpointTrust(tc.url)
			if err == nil {
				t.Fatalf("endpoint %q must be refused", tc.url)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q must mention %q", err, tc.want)
			}
		})
	}
	for _, value := range []string{"", "https://127.0.0.1:8443/", "https://perses.lab-01.baseharbor.test/"} {
		if err := ValidatePersesEndpointTrust(value); err != nil {
			t.Fatalf("endpoint %q must be trusted: %v", value, err)
		}
	}
}

func TestPersesOwnershipErrorIsTheSentinelForUnattributableRequests(t *testing.T) {
	_, err := EvaluatePersesOperatorAccess(context.Background(), PersesAccessRequest{Target: "lab-01", Policy: capability.DefaultPersesSecurityPolicy()})
	if !errors.Is(err, ErrPersesOwnershipUnknown) {
		t.Fatalf("an unattributable request must return the ownership sentinel, got %v", err)
	}
	ctx, request := persesVerifiedRequest("admin")
	request.Target = ""
	if _, err := EvaluatePersesOperatorAccess(ctx, request); !errors.Is(err, ErrPersesOwnershipUnknown) {
		t.Fatalf("an unattributed Target must return the ownership sentinel, got %v", err)
	}
}
