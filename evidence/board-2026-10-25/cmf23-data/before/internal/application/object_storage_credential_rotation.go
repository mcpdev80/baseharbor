package application

import (
	"errors"
	"fmt"
	"strings"
)

// ReplaceObjectStorageCredentials replaces one logical bucket credential pair in
// the managed runtime state. Callers must re-project the object-storage binding
// after this mutation so workloads observe the new credential atomically.
func ReplaceObjectStorageCredentials(m Manifest, files RuntimeFiles, bucket string, credentials ObjectStorageCredentials) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return errors.New("object-storage bucket is required")
	}
	if strings.TrimSpace(credentials.AccessKeyID) == "" || strings.TrimSpace(credentials.SecretAccessKey) == "" {
		return errors.New("replacement object-storage credentials are incomplete")
	}
	found := false
	for _, candidate := range ObjectStorageBucketNames(m) {
		if candidate == bucket {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("object-storage bucket %q is not declared by the application", bucket)
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	values[s3RuntimeKey(bucket, "ACCESS_KEY_ID")] = credentials.AccessKeyID
	values[s3RuntimeKey(bucket, "SECRET_ACCESS_KEY")] = credentials.SecretAccessKey
	return writeRuntimeEnv(files.Env, m, values)
}
