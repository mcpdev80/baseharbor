package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

func requiresDevelopmentGateway(m application.Manifest) bool {
	if !devaccess.Enabled(m.Environment) {
		return false
	}
	return len(m.Exposures) > 0 ||
		application.HasExplicitWorkload(m) ||
		m.Services.Identity ||
		m.Services.SQLManagementUI ||
		m.Services.CacheManagementUI ||
		m.Services.ObjectStorageManagementUI ||
		m.Services.SecretsManagementUI ||
		m.Services.IdentityManagementUI ||
		m.Services.ObservabilityManagementUI
}

func requiresDevelopmentManagementAccess(m application.Manifest) bool {
	if !devaccess.Enabled(m.Environment) {
		return false
	}
	return m.Services.Identity ||
		m.Services.SQLManagementUI ||
		m.Services.CacheManagementUI ||
		m.Services.ObjectStorageManagementUI ||
		m.Services.SecretsManagementUI ||
		m.Services.IdentityManagementUI ||
		m.Services.ObservabilityManagementUI
}

func (e *applicationApplyExecution) reconcileDevelopmentCanonicalRoutes(ctx context.Context) error {
	if !requiresDevelopmentGateway(e.manifest) {
		return nil
	}
	target := e.resolved.Target.Name
	if _, err := devaccess.EnsureDomain(target); err != nil {
		return fmt.Errorf("prepare development domain: %w", err)
	}

	appOwner := "app/" + e.manifest.Name + "/" + e.manifest.Environment
	appRoutes := make([]devgateway.Route, 0, 8)
	groups := []devgateway.OwnerRoutes{}

	if e.manifest.Services.SQLManagementUI {
		host, err := devaccess.ApplicationHost(target, e.manifest.Name, "pgadmin")
		if err != nil {
			return err
		}
		appRoutes = append(appRoutes, devgateway.Route{
			Key: appOwner + "/pgadmin", Host: host,
			Upstream: "https://" + devaccess.ApplicationAlias(e.manifest.Name, "pgadmin") + ":8443",
			Network: application.ApplicationBackendNetworkNameForProject(e.files.ResourceProject),
			TrustFile: filepath.Join(e.files.Dir, "providers", "management-ui", "postgres", "pki", "ca.pem"),
			ServerName: "localhost",
		})
	}
	if e.manifest.Services.CacheManagementUI {
		host, err := devaccess.ApplicationHost(target, e.manifest.Name, "cache")
		if err != nil {
			return err
		}
		appRoutes = append(appRoutes, devgateway.Route{
			Key: appOwner + "/cache", Host: host,
			Upstream: "https://" + devaccess.ApplicationAlias(e.manifest.Name, "cache") + ":8443",
			Network: application.ApplicationBackendNetworkNameForProject(e.files.ResourceProject),
			TrustFile: filepath.Join(e.files.Dir, "providers", "management-ui", "cache", "pki", "ca.pem"),
			ServerName: "localhost",
		})
	}
	if e.manifest.Services.Identity {
		placement, err := application.ResolveProviderPlacement(e.manifest, capability.ProviderKeycloak)
		if err != nil {
			return err
		}
		if placement.Scope != capability.ScopeExternal {
			files, err := identityprovider.ExistingKeycloakFilesAt(e.manifest, e.resolved.TargetStateRoot, target)
			if err != nil {
				return err
			}
			host, err := devaccess.ApplicationHost(target, e.manifest.Name, "identity")
			if err != nil {
				return err
			}
			appRoutes = append(appRoutes, devgateway.Route{
				Key: appOwner + "/identity", Host: host,
				Upstream: fmt.Sprintf("https://%s:%d", devaccess.ProviderAlias(files.Project, "identity"), files.PublicPort),
				Network: files.ConsumerNetwork,
				TrustFile: files.PublicAccess.Material.CA,
				ServerName: files.PublicAccess.Material.ServerName,
			})
			if e.manifest.Services.IdentityManagementUI {
				adminHost, err := devaccess.ApplicationHost(target, e.manifest.Name, "identity-admin")
				if err != nil {
					return err
				}
				appRoutes = append(appRoutes, devgateway.Route{
					Key: appOwner + "/identity-admin", Host: adminHost,
					Upstream: "https://" + devaccess.ProviderAlias(files.Project, "identity-admin") + ":9443",
					Network: files.InternalNetwork,
					TrustFile: files.AdminAccess.Material.CA,
					ServerName: files.AdminAccess.Material.ServerName,
				})
			}
		}
	}
	if len(e.manifest.Exposures) == 0 && application.HasExplicitWorkload(e.manifest) {
		selected, composePath, found, err := application.SelectedWorkloadServices(e.resolved.repositoryRoot(), e.manifest)
		if err != nil {
			return err
		}
		if found && len(selected) == 1 {
			relative := composePath
			if rel, relErr := filepath.Rel(e.resolved.repositoryRoot(), composePath); relErr == nil {
				relative = rel
			}
			analysis, err := repositoryinspect.AnalyzeComposeFile(e.resolved.repositoryRoot(), relative)
			if err != nil {
				return err
			}
			var ports []int
			for _, item := range analysis.Ports {
				if item.Service != selected[0] {
					continue
				}
				if port, ok := composeTargetPort(item.Value); ok {
					ports = append(ports, port)
				}
			}
			if len(ports) == 1 {
				host, err := devaccess.ApplicationHost(target, e.manifest.Name, "api")
				if err != nil {
					return err
				}
				appRoutes = append(appRoutes, devgateway.Route{
					Key: appOwner + "/workload-api",
					Host: host,
					Upstream: fmt.Sprintf("http://%s:%d", application.DevelopmentWorkloadAlias(e.manifest), ports[0]),
					Network: application.DevelopmentWorkloadNetworkNameForProject(e.files.Project),
				})
			}
		}
	}

	if len(e.manifest.Exposures) > 0 {
		state, providerFiles, err := exposure.Load(e.files)
		if err != nil {
			return fmt.Errorf("load managed exposure for canonical routes: %w", err)
		}
		public := make([]exposure.Route, 0, len(state.Routes))
		for _, route := range state.Routes {
			if strings.EqualFold(strings.TrimSpace(route.Visibility), "internal") {
				continue
			}
			public = append(public, route)
		}
		for _, route := range public {
			label := strings.TrimSpace(route.Name)
			if len(public) == 1 {
				label = "api"
			} else if label == "" {
				label = route.Service
			}
			host, err := devaccess.ApplicationHost(target, e.manifest.Name, label)
			if err != nil {
				return err
			}
			containerPort := 8080
			upstreamScheme := "http"
			trustFile := ""
			serverName := ""
			if route.Protocol == "https" {
				containerPort = 8443
				upstreamScheme = "https"
				trustFile = filepath.Join(providerFiles.Dir, "routes", route.Name, "cert.pem")
				serverName = state.Host
			}
			appRoutes = append(appRoutes, devgateway.Route{
				Key: appOwner + "/exposure/" + route.Name,
				Host: host,
				Upstream: fmt.Sprintf("%s://baseharbor-internal-exposure-%s:%d", upstreamScheme, route.Name, containerPort),
				Network: state.Network,
				TrustFile: trustFile,
				ServerName: serverName,
			})
		}
	}
	groups = append(groups, devgateway.OwnerRoutes{Owner: appOwner, Routes: appRoutes})

	if e.manifest.Services.ObjectStorageManagementUI {
		files, err := objectstorage.ExistingProviderFilesAt(e.resolved.TargetStateRoot, target)
		if err != nil {
			return err
		}
		host, err := devaccess.SharedHost(target, "storage")
		if err != nil {
			return err
		}
		groups = append(groups, devgateway.OwnerRoutes{
			Owner: "shared/object-storage",
			Routes: []devgateway.Route{{
				Key: "shared/object-storage", Host: host,
				Upstream: "https://seaweedfs-admin-access:9443",
				Network: files.Network,
				TrustFile: filepath.Join(files.Dir, "management-ui", "service-access", "pki", "ca.pem"),
				ServerName: "localhost",
			}},
		})
	}
	if e.manifest.Services.SecretsManagementUI {
		host, err := devaccess.SharedHost(target, "openbao")
		if err != nil {
			return err
		}
		groups = append(groups, devgateway.OwnerRoutes{
			Owner: "shared/openbao",
			Routes: []devgateway.Route{{
				Key: "shared/openbao", Host: host,
				Upstream: "https://openbao-access:8443",
				Network: e.platformFiles.Project + "_default",
				TrustFile: filepath.Join(filepath.Dir(e.platformFiles.Compose), "providers", "openbao", "service-access", "pki", "ca.pem"),
				ServerName: "localhost",
			}},
		})
	}
	if e.manifest.Services.ObservabilityManagementUI {
		placement, err := metrics.PlacementForAt(e.resolved.TargetStateRoot, target, e.manifest)
		if err != nil {
			return err
		}
		if placement.Scope != capability.ScopeExternal {
			files, err := metrics.ExistingProviderFilesAt(e.resolved.TargetStateRoot, target, e.manifest)
			if err != nil {
				return err
			}
			service := "prometheus-access"
			host, err := devaccess.SharedHost(target, "prometheus")
			owner := "shared/prometheus"
			key := "shared/prometheus"
			if placement.Scope == capability.ScopeApplication {
				service = "baseharbor-internal-prometheus-access"
				host, err = devaccess.ApplicationHost(target, e.manifest.Name, "prometheus")
				owner = appOwner + "/prometheus"
				key = owner
			}
			if err != nil {
				return err
			}
			groups = append(groups, devgateway.OwnerRoutes{
				Owner: owner,
				Routes: []devgateway.Route{{
					Key: key, Host: host,
					Upstream: "https://" + service + ":8443",
					Network: placement.Project + "_publish",
					TrustFile: filepath.Join(files.Dir, "service-access", "pki", "ca.pem"),
					ServerName: "localhost",
				}},
			})
		}
	}

	if err := devgateway.ReplaceRoutes(ctx, e.compose, e.issuer, target, groups...); err != nil {
		return fmt.Errorf("reconcile canonical development routes: %w", err)
	}
	if err := devgateway.Verify(ctx, target); err != nil {
		return fmt.Errorf("verify canonical development routes: %w", err)
	}

	routes, err := devgateway.Routes(target)
	if err != nil {
		return err
	}
	e.term.Section("Development URLs")
	for _, route := range routes {
		if route.Owner == appOwner || strings.HasPrefix(route.Owner, "shared/") {
			e.term.Result("READY", route.Key, devgateway.URL(route.Host))
		}
	}
	return nil
}
