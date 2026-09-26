package objectstorage

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type BackupObject struct {
	Key  string `json:"key"`
	Data []byte `json:"data"`
}

type BucketBackup struct {
	LogicalBucket string         `json:"logical_bucket"`
	Objects       []BackupObject `json:"objects"`
}

func (d *Driver) ExportBucket(ctx context.Context, logical string) (BucketBackup, error) {
	files, err := d.existingProviderFiles()
	if err != nil {
		return BucketBackup{}, err
	}
	credentials, err := application.LoadObjectStorageCredentials(d.files, logical)
	if err != nil {
		return BucketBackup{}, err
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return BucketBackup{}, err
	}
	client, err := s3HTTPClient(files)
	if err != nil {
		return BucketBackup{}, err
	}
	physical := PhysicalBucketName(d.app, logical)
	keys, err := listBucketObjectKeys(ctx, client, endpoint, physical, credentials)
	if err != nil {
		return BucketBackup{}, fmt.Errorf("list S3 bucket %s for recovery: %w", logical, err)
	}
	backup := BucketBackup{LogicalBucket: logical, Objects: make([]BackupObject, 0, len(keys))}
	for _, key := range keys {
		status, body, err := signedS3RequestLimit(ctx, client, endpoint, http.MethodGet, physical, key, credentials, nil, 512<<20)
		if err != nil {
			return BucketBackup{}, fmt.Errorf("read S3 object %s/%s: %w", logical, key, err)
		}
		if status != http.StatusOK {
			return BucketBackup{}, fmt.Errorf("read S3 object %s/%s returned HTTP %d", logical, key, status)
		}
		backup.Objects = append(backup.Objects, BackupObject{Key: key, Data: body})
	}
	return backup, nil
}

func (d *Driver) RestoreBucket(ctx context.Context, backup BucketBackup) error {
	logical := strings.TrimSpace(backup.LogicalBucket)
	if logical == "" {
		return errors.New("S3 recovery bucket name is required")
	}
	files, err := d.existingProviderFiles()
	if err != nil {
		return err
	}
	credentials, err := application.LoadObjectStorageCredentials(d.files, logical)
	if err != nil {
		return err
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return err
	}
	client, err := s3HTTPClient(files)
	if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, logical)
	existing, err := listBucketObjectKeys(ctx, client, endpoint, physical, credentials)
	if err != nil {
		return fmt.Errorf("list S3 bucket %s before restore: %w", logical, err)
	}
	for _, key := range existing {
		status, _, err := signedS3Request(ctx, client, endpoint, http.MethodDelete, physical, key, credentials, nil)
		if err != nil {
			return fmt.Errorf("delete existing S3 object %s/%s: %w", logical, key, err)
		}
		if status != http.StatusNoContent && status != http.StatusOK {
			return fmt.Errorf("delete existing S3 object %s/%s returned HTTP %d", logical, key, status)
		}
	}
	seen := map[string]struct{}{}
	for _, object := range backup.Objects {
		key := strings.TrimSpace(object.Key)
		if key == "" {
			return errors.New("S3 recovery object key is required")
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate S3 recovery object %q", key)
		}
		seen[key] = struct{}{}
		status, _, err := signedS3Request(ctx, client, endpoint, http.MethodPut, physical, key, credentials, object.Data)
		if err != nil {
			return fmt.Errorf("restore S3 object %s/%s: %w", logical, key, err)
		}
		if status != http.StatusOK && status != http.StatusNoContent {
			return fmt.Errorf("restore S3 object %s/%s returned HTTP %d", logical, key, status)
		}
	}
	keys, err := listBucketObjectKeys(ctx, client, endpoint, physical, credentials)
	if err != nil {
		return fmt.Errorf("verify restored S3 bucket %s: %w", logical, err)
	}
	sort.Strings(keys)
	want := make([]string, 0, len(backup.Objects))
	for _, object := range backup.Objects {
		want = append(want, object.Key)
	}
	sort.Strings(want)
	if strings.Join(keys, "\x00") != strings.Join(want, "\x00") {
		return fmt.Errorf("verify restored S3 bucket %s: object set differs from recovery unit", logical)
	}
	for _, object := range backup.Objects {
		status, body, err := signedS3RequestLimit(ctx, client, endpoint, http.MethodGet, physical, object.Key, credentials, nil, int64(len(object.Data))+1)
		if err != nil || status != http.StatusOK || string(body) != string(object.Data) {
			return fmt.Errorf("verify restored S3 object %s/%s failed", logical, object.Key)
		}
	}
	return nil
}

func listBucketObjectKeys(ctx context.Context, client *http.Client, endpoint, bucket string, credentials application.ObjectStorageCredentials) ([]string, error) {
	var keys []string
	continuation := ""
	for {
		query := url.Values{"list-type": {"2"}, "max-keys": {"1000"}}
		if continuation != "" {
			query.Set("continuation-token", continuation)
		}
		status, body, err := signedS3RequestQuery(ctx, client, endpoint, http.MethodGet, bucket, "", query, credentials, nil, 8<<20)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("ListObjectsV2 returned HTTP %d", status)
		}
		var response struct {
			Contents []struct {
				Key string `xml:"Key"`
			} `xml:"Contents"`
			Truncated bool   `xml:"IsTruncated"`
			Next      string `xml:"NextContinuationToken"`
		}
		if err := xml.Unmarshal(body, &response); err != nil {
			return nil, errors.New("decode S3 ListObjectsV2 response")
		}
		for _, item := range response.Contents {
			if item.Key != "" {
				keys = append(keys, item.Key)
			}
		}
		if !response.Truncated {
			break
		}
		if strings.TrimSpace(response.Next) == "" || response.Next == continuation {
			return nil, errors.New("S3 ListObjectsV2 pagination did not advance")
		}
		continuation = response.Next
	}
	sort.Strings(keys)
	return keys, nil
}
