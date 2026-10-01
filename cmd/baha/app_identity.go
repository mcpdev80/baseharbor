package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type managedIdentityExecution struct {
	execution *capability.Execution
	keycloak  *identityprovider.KeycloakDriver
	provider  capability.ProviderKind
	target    string
	manifest  application.Manifest
}

func prepareManagedIdentity(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, issuer serviceaccess.Issuer) (*managedIdentityExecution, error) {
	m := resolved.Manifest
	if !application.HasIdentity(m) {
		return nil, nil
	}
	bindings, err := application.CapabilityBindings(m)
	if err != nil {
		return nil, err
	}
	var identity capability.Binding
	found := false
	for _, binding := range bindings {
		if binding.Resource.Kind == capability.Identity {
			identity = binding
			found = true
			break
		}
	}
	if !found || identity.Identity == nil {
		return nil, fmt.Errorf("identity capability binding is missing")
	}

	files := application.RuntimeFilesFor(resolved.Store, m)
	var driver capability.Driver
	prepared := &managedIdentityExecution{
		provider: identity.Resource.Provider,
		target:   resolved.Target.Name,
		manifest: m,
	}
	switch identity.Resource.Provider {
	case capability.ProviderKeycloak:
		keycloak := identityprovider.NewKeycloakDriver(compose, m, files, issuer, resolved.TargetStateRoot, resolved.Target.Name)
		driver = keycloak
		prepared.keycloak = keycloak
	case capability.ProviderExternalOIDC:
		placement, err := application.ResolveProviderPlacement(m, capability.ProviderExternalOIDC)
		if err != nil {
			return nil, err
		}
		external, err := identityprovider.NewExternalDriver(m, files, placement.ExternalReference)
		if err != nil {
			return nil, err
		}
		driver = external
	default:
		return nil, fmt.Errorf("unsupported identity provider %q", identity.Resource.Provider)
	}

	request := capability.Request{
		Requirement: capability.Requirement{Kind: capability.Identity, Name: identity.Resource.Name},
		Workload:    identity.Workload,
		Identity:    identity.Identity,
		Driver:      driver,
	}
	execution, _, err := capability.Prepare(ctx, m.Name, []capability.Request{request})
	if err != nil {
		return nil, err
	}
	prepared.execution = execution
	return prepared, nil
}

func provisionAndVerifyManagedIdentity(ctx context.Context, out io.Writer, prepared *managedIdentityExecution, exposure *managedExposureExecution) error {
	if prepared == nil {
		return nil
	}
	if prepared.keycloak != nil {
		origins, err := managedIdentityExposureOrigins(prepared, exposure)
		if err != nil {
			return err
		}
		if err := prepared.keycloak.SetApplicationOrigins(origins); err != nil {
			return err
		}
	}
	if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
		return err
	}
	if _, err := prepared.execution.Verify(ctx); err != nil {
		return err
	}
	fmt.Fprintf(out, "[READY] identity provider=%s standard=OIDC/OAuth2\n", prepared.provider)
	return nil
}

func managedIdentityExposureOrigins(identity *managedIdentityExecution, prepared *managedExposureExecution) ([]string, error) {
	if prepared == nil {
		return nil, nil
	}
	state := prepared.driver.State()
	seen := map[string]struct{}{}
	var origins []string
	publicCount := 0
	for _, route := range state.Routes {
		if !strings.EqualFold(route.Visibility, "internal") {
			publicCount++
		}
	}
	for _, route := range state.Routes {
		if strings.EqualFold(route.Visibility, "internal") {
			continue
		}
		var origin string
		if identity != nil && devaccess.Enabled(identity.manifest.Environment) {
			service := devaccess.ExposureService(route.Name, publicCount)
			host, err := devaccess.ApplicationHost(identity.target, identity.manifest.Name, service)
			if err != nil {
				return nil, err
			}
			origin = devaccess.CanonicalURL(host)
		} else {
			host := strings.TrimSpace(state.Host)
			if host == "" || route.PublishedPort < 1 {
				return nil, fmt.Errorf("managed application exposure is incomplete for identity redirect resolution")
			}
			origin = route.Protocol + "://" + net.JoinHostPort(host, strconv.Itoa(route.PublishedPort))
		}
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins, nil
}

func verifyExistingManagedIdentity(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, issuer serviceaccess.Issuer) error {
	m := resolved.Manifest
	if !application.HasIdentity(m) {
		return nil
	}
	bindings, err := application.CapabilityBindings(m)
	if err != nil {
		return err
	}
	var identityBinding capability.Binding
	found := false
	for _, binding := range bindings {
		if binding.Resource.Kind == capability.Identity {
			identityBinding = binding
			found = true
			break
		}
	}
	if !found || identityBinding.Identity == nil {
		return fmt.Errorf("identity capability binding is missing")
	}

	files := application.RuntimeFilesFor(resolved.Store, m)
	switch identityBinding.Resource.Provider {
	case capability.ProviderKeycloak:
		var origins []string
		if len(m.Exposures) > 0 {
			state, _, err := exposure.Load(files)
			if err != nil {
				return fmt.Errorf("load managed exposure for identity verification: %w", err)
			}
			seen := map[string]struct{}{}
			publicCount := 0
			for _, route := range state.Routes {
				if !strings.EqualFold(route.Visibility, "internal") {
					publicCount++
				}
			}
			for _, route := range state.Routes {
				if strings.EqualFold(route.Visibility, "internal") {
					continue
				}
				origin := ""
				if devaccess.Enabled(m.Environment) {
					service := devaccess.ExposureService(route.Name, publicCount)
					host, err := devaccess.ApplicationHost(resolved.Target.Name, m.Name, service)
					if err != nil {
						return err
					}
					origin = devaccess.CanonicalURL(host)
				} else {
					origin = route.Protocol + "://" + net.JoinHostPort(state.Host, strconv.Itoa(route.PublishedPort))
				}
				if _, ok := seen[origin]; ok {
					continue
				}
				seen[origin] = struct{}{}
				origins = append(origins, origin)
			}
		}
		driver := identityprovider.NewKeycloakDriver(compose, m, files, issuer, resolved.TargetStateRoot, resolved.Target.Name)
		return driver.VerifyExisting(ctx, identityBinding, origins)
	case capability.ProviderExternalOIDC:
		placement, err := application.ResolveProviderPlacement(m, capability.ProviderExternalOIDC)
		if err != nil {
			return err
		}
		driver, err := identityprovider.NewExternalDriver(m, files, placement.ExternalReference)
		if err != nil {
			return err
		}
		return driver.Verify(ctx, identityBinding.Resource, identityBinding)
	default:
		return fmt.Errorf("unsupported identity provider %q", identityBinding.Resource.Provider)
	}
}
