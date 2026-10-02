package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func ManagementUISurfaceAt(dataDir, namespace string, m application.Manifest) (application.ManagementUISurface, error) {
	if !m.Services.ObservabilityManagementUI {
		return application.ManagementUISurface{}, fmt.Errorf("observability management UI is not selected")
	}
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return application.ManagementUISurface{}, err
	}
	if placement.Scope == capability.ScopeExternal {
		return application.ManagementUISurface{}, fmt.Errorf("external metrics provider has no BaseHarbor-owned management UI")
	}
	files, err := ExistingProviderFilesAt(dataDir, namespace, m)
	if err != nil {
		return application.ManagementUISurface{}, err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return application.ManagementUISurface{}, err
	}
	policy, err := serviceaccess.Resolve(prometheusAccessEnvironment(m, nil), "prometheus", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return application.ManagementUISurface{}, err
	}
	authentication := "none"
	authClass := application.ManagementAuthUnsupported
	roleMappings := []application.ManagementRoleMapping(nil)
	if policy.AuthenticationRequired {
		authentication = string(policy.Authentication)
		authClass = application.ManagementAuthNativeCredential
		roleMappings = application.NativeCredentialRoleMappings("operator", "reader")
	}
	return application.ManagementUISurface{
		Service:        "observability",
		Purpose:        application.ProviderInterfaceObservability,
		URL:            strings.TrimRight(endpoint, "/") + "/",
		Authentication: authentication,
		AuthenticationClass: authClass,
		RoleMappings: roleMappings,
	}, nil
}

func VerifyManagementUIAt(ctx context.Context, dataDir, namespace string, m application.Manifest) error {
	if !m.Services.ObservabilityManagementUI {
		return nil
	}
	files, err := ExistingProviderFilesAt(dataDir, namespace, m)
	if err != nil {
		return err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	client, err := providerHTTPClient(m, files)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/graph", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Prometheus management UI is not reachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("Prometheus management UI returned HTTP %d", resp.StatusCode)
	}
	return nil
}
