package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Service access does not imply a management installation. Reuse a selected
// installation's existing issuer; otherwise realize the same issuer contract
// locally without adding a SQL, Secrets or Identity runtime service.
func resolveApplicationServiceIssuer(ctx context.Context, resolved resolvedApplication, workload bhruntime.RuntimeProvider) (serviceaccess.Issuer, error) {
	selected := withTargetOverride(ctx, resolved.Target.Name)
	target, remote, err := applicationCoreTarget(selected)
	if err != nil {
		return nil, err
	}
	current, err := effectiveTarget(selected)
	if err != nil {
		return nil, err
	}
	if current != resolved.Target {
		return nil, errors.New("application Target binding changed during execution")
	}
	dir := filepath.Join(resolved.TargetStateRoot, "service-access", "issuer")
	if !remote {
		if _, err := os.Lstat(dir); err == nil {
			if err := serviceaccess.CheckLocalIssuer(dir); err != nil {
				return nil, err
			}
			return &serviceaccess.LocalIssuer{Dir: dir}, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(root); err == nil || remote {
		provider, files, err := resolveApplicationCoreRuntime(ctx, resolved, workload)
		if err != nil {
			return nil, err
		}
		issuer := openbao.NewServiceIssuer(provider, files)
		status, err := issuer.Status(ctx)
		if err != nil {
			return nil, err
		}
		if !status.Ready {
			return nil, unavailableApplicationCoreRuntime()
		}
		return issuer, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := serviceaccess.CheckLocalIssuer(dir); err != nil {
		return nil, err
	}
	return &serviceaccess.LocalIssuer{Dir: dir}, nil
}

// Only the authorized mutation phase initializes new authority. All plan and
// preflight callers above remain read-only and preserve damaged state.
func materializeApplicationServiceIssuer(issuer serviceaccess.Issuer) error {
	if local, ok := issuer.(*serviceaccess.LocalIssuer); ok {
		_, err := serviceaccess.EnsureLocalIssuer(local.Dir)
		return err
	}
	return nil
}
