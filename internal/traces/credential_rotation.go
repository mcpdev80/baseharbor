package traces

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RotateStorageCredentials rotates Tempo's provider-owned S3 identity using the
// same overlap-safe platform-bucket contract as Loki.
func (d *Driver) RotateStorageCredentials(ctx context.Context) error {
	if !d.app.HA {
		return errors.New("Tempo platform-storage credential rotation requires HA mode")
	}
	if d.runtime == nil || d.issuer == nil {
		return errors.New("Tempo platform-storage credential rotation requires managed runtime and issuer")
	}
	storageRuntime, ok := d.runtime.(objectstorage.Runtime)
	if !ok {
		return errors.New("Tempo platform-storage credential rotation requires runtime object-storage administration support")
	}
	dataDir := strings.TrimSpace(d.dataDir)
	if dataDir == "" || dataDir == "." {
		var err error
		dataDir, err = bhruntime.DataDir("")
		if err != nil {
			return err
		}
	}
	placement, err := PlacementForAt(dataDir, d.namespace, d.app)
	if err != nil {
		return err
	}

	reconcile := func(ctx context.Context, bucket objectstorage.PlatformBucket) error {
		files, _, err := ensureProviderFilesAt(ctx, d.issuer, dataDir, d.namespace, d.app, &bucket)
		if err != nil {
			return err
		}
		if err := d.runtime.ConfigProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
			return err
		}
		return d.runtime.UpProject(ctx, placement.Project, files.Compose, files.Env)
	}
	verify := func(ctx context.Context, _ objectstorage.PlatformBucket) error {
		files, _, err := ExistingProviderFilesAt(dataDir, d.namespace, d.app)
		if err != nil {
			return err
		}
		client, err := tempoHTTPClient(d.app, files)
		if err != nil {
			return err
		}
		endpoint, err := ProviderEndpoint(files)
		if err != nil {
			return err
		}
		if err := waitReady(ctx, client, endpoint); err != nil {
			return fmt.Errorf("verify Tempo after platform credential rotation: %w", err)
		}
		return nil
	}
	return objectstorage.RotatePlatformBucketCredentialsAt(
		ctx,
		storageRuntime,
		d.issuer,
		filepath.Clean(dataDir),
		d.namespace,
		"tempo",
		objectstorage.PlatformBucketRotationHooks{
			Reconcile: reconcile,
			Verify:    verify,
			Rollback:  reconcile,
		},
	)
}


// RotateAccessPKI rotates Tempo's stable HTTPS/mTLS query identity with
// overlap-safe trust replacement and explicit old-CA retirement.
func (d *Driver) RotateAccessPKI(ctx context.Context) error {
	if d.runtime == nil || d.issuer == nil {
		return errors.New("Tempo access PKI rotation requires managed runtime and issuer")
	}
	dataDir := strings.TrimSpace(d.dataDir)
	if dataDir == "" || dataDir == "." {
		var err error
		dataDir, err = bhruntime.DataDir("")
		if err != nil {
			return err
		}
	}
	placement, err := PlacementForAt(dataDir, d.namespace, d.app)
	if err != nil {
		return err
	}
	files, _, err := ExistingProviderFilesAt(dataDir, d.namespace, d.app)
	if err != nil {
		return err
	}
	policy, err := serviceaccess.Resolve(d.app.Environment, "tempo", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return err
	}
	spec := tempoAccessSpec()
	if d.app.HA {
		spec = tempoHAQueryAccessSpec()
	}
	reconcile := func() error {
		if _, err := serviceaccess.EnsureHTTPGateway(ctx, d.issuer, policy, files.Dir, spec); err != nil {
			return err
		}
		if err := d.runtime.ConfigProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
			return err
		}
		if err := d.runtime.UpProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
			return err
		}
		client, err := tempoHTTPClient(d.app, files)
		if err != nil {
			return err
		}
		endpoint, err := ProviderEndpoint(files)
		if err != nil {
			return err
		}
		verifyCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if err := waitReady(verifyCtx, client, endpoint); err != nil {
			return fmt.Errorf("verify Tempo after PKI reconcile: %w", err)
		}
		return nil
	}
	if err := reconcile(); err != nil {
		return fmt.Errorf("reconcile replacement Tempo PKI with overlap: %w", err)
	}
	if err := serviceaccess.RetireTLSOverlap(ctx, d.issuer, policy, filepath.Join(files.Dir, "service-access", "pki")); err != nil {
		return fmt.Errorf("retire previous Tempo CA: %w", err)
	}
	if err := reconcile(); err != nil {
		return fmt.Errorf("reconcile Tempo after CA retirement: %w", err)
	}
	return nil
}
