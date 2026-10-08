package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// inspectIsolatedCoreProviders enumerates registered, applied application projects.
// No name-prefix sweep can authorize a foreign container for a Core update.
func inspectIsolatedCoreProviders(ctx context.Context, rt bhruntime.RuntimeProvider, target string, catalog coreupdate.ReleaseManifest) ([]coreupdate.Delta, error) {
	records, err := deployment.ListDeployments(target)
	if err != nil {
		return nil, fmt.Errorf("inspect protected deployment registry: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	containers, err := rt.ListRuntimeContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("inspect application-scoped runtime containers: %w", err)
	}
	desired := catalog.Providers
	seen := map[string]bool{}
	result := []coreupdate.Delta{}
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if record.Identity.Target != target {
			return nil, errors.New("deployment registry returned different owning Target")
		}
		if len(record.Applied.Intent) == 0 {
			return nil, fmt.Errorf("deployment %s lacks protected applied intent", record.Identity.DeploymentID)
		}
		var m application.Manifest
		if err := json.Unmarshal(record.Applied.Intent, &m); err != nil {
			return nil, fmt.Errorf("decode protected intent %s: %w", record.Identity.DeploymentID, err)
		}
		if m.Name != record.Identity.Application || m.Environment != record.Identity.Environment || m.ApplicationID != record.Identity.ApplicationID {
			return nil, fmt.Errorf("deployment %s applied intent does not match owned deployment identity", record.Identity.DeploymentID)
		}
		files := application.RuntimeFilesFor(application.Store{Namespace: target}, m)
		project := files.Project
		for _, container := range containers {
			if container.Project != project {
				continue
			}
			kind, ok := isolatedCoreServiceKind(container.Service)
            if !ok || !manifestAuthorizesIsolatedProvider(m,kind,container.Service){
                continue
            }
			if !container.Running {
				return nil, fmt.Errorf("registered isolated %s service %s/%s is not running", kind, project, container.Service)
			}
			key := project + "/" + container.Service
			if seen[key] {
				return nil, fmt.Errorf("duplicate isolated provider ownership %s", key)
			}
			seen[key] = true
			image, err := rt.ProjectServiceImageIdentity(ctx, project, container.Service)
			if err != nil {
				return nil, fmt.Errorf("inspect isolated %s/%s: %w", project, container.Service, err)
			}
			ref := strings.TrimSpace(image.Reference)
			tag, err := coreProviderImageVersion(ref, kind)
			if err != nil {
				return nil, fmt.Errorf("isolated %s/%s: %w", project, container.Service, err)
			}
			digest := strings.TrimSpace(image.Digest)
			if i := strings.Index(digest, "@sha256:"); i >= 0 {
				digest = digest[i+1:]
			}
			realization := coreupdate.Realization{
				Kind: kind, Installation: record.Identity.DeploymentID, Scope: "application", Instance: container.Service,
				Owner: "baseharbor", Image: ref, Version: tag, Digest: digest,
			}
			partial, err := coreupdate.Build(catalog.Release, []coreupdate.Realization{realization}, desired)
			if err != nil {
				return nil, err
			}
			if len(partial.Deltas) != 1 {
				return nil, fmt.Errorf("isolated provider %s did not produce exact realization", key)
			}
			result = append(result, partial.Deltas[0])
		}
	}
	return result, nil
}

func isolatedCoreServiceKind(service string) (coreupdate.ProviderKind, bool) {
	switch {
	case service == "postgres", strings.HasPrefix(service, "postgres-") && service != "postgres-access" && service != "postgres-exporter" && service != "postgres-ui":
		return coreupdate.SQL, true
	case service == "openbao", strings.HasPrefix(service, "openbao-member-"), strings.HasPrefix(service, "openbao-instance-"):
		return coreupdate.Secrets, true
	case service == "keycloak", strings.HasPrefix(service, "keycloak-member-"), strings.HasPrefix(service, "keycloak-instance-"):
		return coreupdate.Identity, true
	default:
		return "", false
	}
}

func coreProviderImageVersion(ref string, kind coreupdate.ProviderKind) (string, error) {
	pos := strings.LastIndex(ref, ":")
	if pos <= strings.LastIndex(ref, "/") || pos == len(ref)-1 {
		return "", errors.New("image lacks explicit immutable reference tag")
	}
	version := ref[pos+1:]
	if kind == coreupdate.SQL {
		version = strings.SplitN(version, "-", 2)[0]
	}
	return version, nil
}

func manifestAuthorizesIsolatedProvider(m application.Manifest,kind coreupdate.ProviderKind,service string)bool{
 switch kind{
 case coreupdate.SQL:
  for _,instance:=range application.SQLInstanceNames(m){
   base:="postgres"
   if instance!="default"{base+="-"+instance}
   if service==base||strings.HasPrefix(service,base+"-member-"){return true}
  }
 case coreupdate.Secrets:
  return m.Services.Secrets && (service=="openbao"||strings.HasPrefix(service,"openbao-member-"))
 case coreupdate.Identity:
  return m.Services.Identity && (service=="keycloak"||strings.HasPrefix(service,"keycloak-member-"))
 }
 return false
}
