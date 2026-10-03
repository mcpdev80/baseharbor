package logs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// RotateStorageCredentials rotates Loki's provider-owned S3 identity without
// removing healthy capacity. The replacement IAM identity is reconciled into
// the Loki runtime, the stable query endpoint is verified, and only then is the
// previous SeaweedFS identity retired.
func (d *Driver) RotateStorageCredentials(ctx context.Context) error {
	if !d.app.HA {
		return errors.New("Loki platform-storage credential rotation requires HA mode")
	}
	if d.runtime == nil || d.issuer == nil {
		return errors.New("Loki platform-storage credential rotation requires managed runtime and issuer")
	}
	storageRuntime, ok := d.runtime.(objectstorage.Runtime)
	if !ok {
		return errors.New("Loki platform-storage credential rotation requires runtime object-storage administration support")
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
	mode := logCollectionMode(d.runtime)

	reconcile := func(ctx context.Context, bucket objectstorage.PlatformBucket) error {
		files, err := ensureProviderFilesForModeAt(ctx, d.issuer, dataDir, d.namespace, d.app, mode, &bucket)
		if err != nil {
			return err
		}
		if err := d.runtime.ConfigProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
			return err
		}
		return d.runtime.UpProject(ctx, placement.Project, files.Compose, files.Env)
	}
	verify := func(ctx context.Context, _ objectstorage.PlatformBucket) error {
		files, err := ExistingProviderFilesAt(dataDir, d.namespace, d.app)
		if err != nil {
			return err
		}
		client, err := lokiHTTPClient(d.app, files)
		if err != nil {
			return err
		}
		endpoint, err := ProviderEndpoint(files)
		if err != nil {
			return err
		}
		if err := serviceaccess.WaitHTTPS(ctx, client, endpoint, "/ready"); err != nil {
			return fmt.Errorf("verify Loki after platform credential rotation: %w", err)
		}
		return nil
	}
	return objectstorage.RotatePlatformBucketCredentialsAt(
		ctx,
		storageRuntime,
		d.issuer,
		filepath.Clean(dataDir),
		d.namespace,
		"loki",
		objectstorage.PlatformBucketRotationHooks{
			Reconcile: reconcile,
			Verify:    verify,
			Rollback:  reconcile,
		},
	)
}


// RotateAccessPKI rotates Loki's stable HTTPS/mTLS access identity with
// old+new trust overlap, verifies the replacement path, retires the old CA,
// reprojects new-only material, and verifies the stable endpoint again.
func (d *Driver) RotateAccessPKI(ctx context.Context) error {
	if d.runtime == nil || d.issuer == nil {
		return errors.New("Loki access PKI rotation requires managed runtime and issuer")
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
	files, err := ExistingProviderFilesAt(dataDir, d.namespace, d.app)
	if err != nil {
		return err
	}
	registrations, err := readRegistrations(files.Registrations)
	if err != nil {
		return err
	}
	policy, err := serviceaccess.Resolve(lokiAccessEnvironment(d.app, registrations), "loki", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return err
	}
	spec := lokiAccessSpec()
	if d.app.HA {
		spec.Upstream = ""
		spec.Upstreams = []string{"http://loki-1:3100", "http://loki-2:3100", "http://loki-3:3100"}
		spec.NetworkAliases = []string{"loki"}
		spec.CertificateNames = []string{"loki"}
	}
	if placement.Scope == capability.ScopeApplication {
		spec.ServiceName = "baseharbor-internal-loki-access"
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
		client, err := lokiHTTPClient(d.app, files)
		if err != nil {
			return err
		}
		endpoint, err := ProviderEndpoint(files)
		if err != nil {
			return err
		}
		verifyCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if err := serviceaccess.WaitHTTPS(verifyCtx, client, endpoint, "/loki/api/v1/status/buildinfo"); err != nil {
			return fmt.Errorf("verify Loki after PKI reconcile: %w", err)
		}
		return nil
	}
	if err := reconcile(); err != nil {
		return fmt.Errorf("reconcile replacement Loki PKI with overlap: %w", err)
	}
	if err := serviceaccess.RetireTLSOverlap(ctx, d.issuer, policy, filepath.Join(files.Dir, "service-access", "pki")); err != nil {
		return fmt.Errorf("retire previous Loki CA: %w", err)
	}
	if err := reconcile(); err != nil {
		return fmt.Errorf("reconcile Loki after CA retirement: %w", err)
	}
	return nil
}
