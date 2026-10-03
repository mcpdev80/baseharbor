package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// RotateProviderPKIAt reconciles SeaweedFS service and management TLS material
// against a replacement issuer, verifies both stable endpoints while the old
// and new trust roots overlap, and retires the previous trust root only after
// the replacement path is proven healthy.
func RotateProviderPKIAt(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, dataDir, namespace string) error {
	if runtime == nil {
		return errors.New("SeaweedFS runtime is required")
	}
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		return err
	}
	values, err := readProviderValues(files.Env)
	if err != nil {
		return err
	}
	managementUI := values[seaweedAdminPortEnv] != ""

	// Reconcile new leaves and overlap trust into the stable runtime projection.
	if _, err := EnsureProviderFilesAt(ctx, issuer, dataDir, namespace); err != nil {
		return fmt.Errorf("reconcile SeaweedFS replacement PKI: %w", err)
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate SeaweedFS replacement PKI: %w", err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("apply SeaweedFS replacement PKI: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, providerReadinessTimeout)
	defer cancel()
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return err
	}
	client, err := s3HTTPClient(files)
	if err != nil {
		return err
	}
	if err := waitS3(probeCtx, client, endpoint); err != nil {
		return fmt.Errorf("verify SeaweedFS replacement service PKI: %w", err)
	}
	if managementUI {
		if err := VerifyManagementUIAt(probeCtx, dataDir, namespace); err != nil {
			return fmt.Errorf("verify SeaweedFS replacement management PKI: %w", err)
		}
	}

	servicePolicy, err := serviceaccess.Resolve("prod", "seaweedfs", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	servicePolicy.ServerName = "seaweedfs"
	if err := serviceaccess.RetireTLSOverlap(ctx, issuer, servicePolicy, filepath.Join(files.Dir, "service-access", "pki")); err != nil {
		return fmt.Errorf("retire previous SeaweedFS service CA: %w", err)
	}
	if managementUI {
		adminPolicy, err := serviceaccess.Resolve("prod", "seaweedfs-admin", serviceaccess.AuthenticationNative)
		if err != nil {
			return err
		}
		if err := serviceaccess.RetireTLSOverlap(ctx, issuer, adminPolicy, filepath.Join(files.Dir, "management-ui", "service-access", "pki")); err != nil {
			return fmt.Errorf("retire previous SeaweedFS management CA: %w", err)
		}
	}

	// Re-project the retired new-only trust bundle and prove continuity again.
	if _, err := EnsureProviderFilesAt(ctx, issuer, dataDir, namespace); err != nil {
		return fmt.Errorf("project retired SeaweedFS PKI: %w", err)
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return err
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return err
	}
	client, err = s3HTTPClient(files)
	if err != nil {
		return err
	}
	postRetireCtx, postRetireCancel := context.WithTimeout(ctx, providerReadinessTimeout)
	defer postRetireCancel()
	if err := waitS3(postRetireCtx, client, endpoint); err != nil {
		return fmt.Errorf("verify SeaweedFS PKI after CA retirement: %w", err)
	}
	if managementUI {
		if err := VerifyManagementUIAt(postRetireCtx, dataDir, namespace); err != nil {
			return fmt.Errorf("verify SeaweedFS management PKI after CA retirement: %w", err)
		}
	}
	return nil
}
