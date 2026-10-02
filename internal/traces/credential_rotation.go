package traces

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/objectstorage"
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
