package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type managementUIBrowserResult struct {
	Name string
	URL  string
	Err  error
}

func verifyIdentityManagementBrowserSurfaces(ctx context.Context, m application.Manifest, targetStateRoot, target string) []managementUIBrowserResult {
	failBoth := func(err error) []managementUIBrowserResult {
		return []managementUIBrowserResult{
			{Name: "identity-login", Err: err},
			{Name: "identity-admin", Err: err},
		}
	}

	files, err := identityprovider.ExistingKeycloakFilesAt(m, targetStateRoot, target)
	if err != nil {
		return failBoth(err)
	}

	if !devaccess.Enabled(m.Environment) {
		loginClient, loginErr := serviceaccess.NewHTTPClient(files.PublicAccess.Material, false)
		if loginErr == nil {
			loginErr = serviceaccess.VerifyBrowserSurface(ctx, loginClient, files.PublicURL+"/")
		}
		adminClient, adminErr := serviceaccess.NewHTTPClient(files.AdminAccess.Material, false)
		if adminErr == nil {
			adminErr = serviceaccess.VerifyBrowserSurface(ctx, adminClient, files.AdminURL+"/")
		}
		return []managementUIBrowserResult{
			{Name: "identity-login", URL: files.PublicURL, Err: loginErr},
			{Name: "identity-admin", URL: files.AdminURL, Err: adminErr},
		}
	}

	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		return failBoth(err)
	}
	if placement.Scope == capability.ScopeExternal {
		return failBoth(fmt.Errorf("external OIDC has no BaseHarbor-managed identity management UI"))
	}

	hostFor := func(service string) (string, error) {
		if placement.Scope == capability.ScopeShared {
			return devaccess.SharedHost(target, service)
		}
		return devaccess.ApplicationHost(target, m.Name, service)
	}
	loginHost, err := hostFor("identity")
	if err != nil {
		return failBoth(err)
	}
	adminHost, err := hostFor("identity-admin")
	if err != nil {
		return failBoth(err)
	}

	gatewayFiles, err := devgateway.FilesFor(target)
	if err != nil {
		return failBoth(err)
	}
	loginURL := devgateway.URLForTarget(target, loginHost)
	adminURL := devgateway.URLForTarget(target, adminHost)
	clientForGateway := func() (*http.Client, error) {
		return serviceaccess.NewHTTPClient(serviceaccess.TLSMaterial{CA: gatewayFiles.CA}, false)
	}

	loginClient, loginErr := clientForGateway()
	if loginErr == nil {
		loginErr = serviceaccess.VerifyBrowserSurface(ctx, loginClient, loginURL+"/")
	}
	adminClient, adminErr := clientForGateway()
	if adminErr == nil {
		adminErr = serviceaccess.VerifyBrowserSurfaceWithAllowedAuthorities(ctx, adminClient, adminURL+"/", loginURL)
	}

	return []managementUIBrowserResult{
		{Name: "identity-login", URL: loginURL, Err: loginErr},
		{Name: "identity-admin", URL: adminURL, Err: adminErr},
	}
}
