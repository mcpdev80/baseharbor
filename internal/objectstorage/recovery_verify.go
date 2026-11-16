package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// HA filer metadata is replicated asynchronously. Successful writes must be
// followed by a bounded convergence check of both the complete key set and
// every object's bytes; a stale list cannot establish restore success.
func waitRestoredBucket(ctx context.Context, client *http.Client, endpoint, physical string, credentials application.ObjectStorageCredentials, backup BucketBackup) error {
	verifyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := verifyRestoredBucket(verifyCtx, client, endpoint, physical, credentials, backup)
		if err == nil {
			return nil
		}
		select {
		case <-verifyCtx.Done():
			return fmt.Errorf("restored S3 bucket %s did not converge: %w", backup.LogicalBucket, errors.Join(verifyCtx.Err(), err))
		case <-ticker.C:
		}
	}
}

func verifyRestoredBucket(ctx context.Context, client *http.Client, endpoint, physical string, credentials application.ObjectStorageCredentials, backup BucketBackup) error {
	keys, err := listBucketObjectKeys(ctx, client, endpoint, physical, credentials)
	if err != nil {
		return fmt.Errorf("verify restored S3 bucket %s: %w", backup.LogicalBucket, err)
	}
	sort.Strings(keys)
	want := make([]string, 0, len(backup.Objects))
	for _, object := range backup.Objects {
		want = append(want, object.Key)
	}
	sort.Strings(want)
	if strings.Join(keys, "\x00") != strings.Join(want, "\x00") {
		return fmt.Errorf("verify restored S3 bucket %s: object set differs from recovery unit", backup.LogicalBucket)
	}
	for _, object := range backup.Objects {
		status, body, err := signedS3RequestLimit(ctx, client, endpoint, http.MethodGet, physical, object.Key, credentials, nil, int64(len(object.Data))+1)
		if err != nil || status != http.StatusOK || string(body) != string(object.Data) {
			return fmt.Errorf("verify restored S3 object %s/%s failed", backup.LogicalBucket, object.Key)
		}
	}
	return nil
}
