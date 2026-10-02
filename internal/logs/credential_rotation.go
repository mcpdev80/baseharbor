package logs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

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
