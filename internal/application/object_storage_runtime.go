package application

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ObjectStorageCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

func LoadObjectStorageCredentials(files RuntimeFiles, bucket string) (ObjectStorageCredentials, error) {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return ObjectStorageCredentials{}, err
	}
	access := values[s3RuntimeKey(bucket, "ACCESS_KEY_ID")]
	secret := values[s3RuntimeKey(bucket, "SECRET_ACCESS_KEY")]
	if access == "" || secret == "" {
		return ObjectStorageCredentials{}, fmt.Errorf("object-storage credentials for %s are not materialized", bucket)
	}
	return ObjectStorageCredentials{AccessKeyID: access, SecretAccessKey: secret}, nil
}

// MaterializeObjectStorageBinding updates the standard application-facing
// environment/file binding after the selected provider has materialized its
// concrete endpoint and bucket identity.
func MaterializeObjectStorageBinding(m Manifest, files RuntimeFiles, logicalBucket, physicalBucket, endpoint string) error {
	credentials, err := LoadObjectStorageCredentials(files, logicalBucket)
	if err != nil {
		return err
	}
	endpoint = strings.TrimSpace(endpoint)
	physicalBucket = strings.TrimSpace(physicalBucket)
	if endpoint == "" || physicalBucket == "" {
		return fmt.Errorf("object-storage binding for %s is incomplete", logicalBucket)
	}
	bindingsAbs, err := filepath.Abs(files.Bindings)
	if err != nil {
		return fmt.Errorf("resolve object-storage bindings directory: %w", err)
	}
	binding, _, err := ensureInstanceBindingDirs(files.Bindings, bindingsAbs, "object-storage-s3", logicalBucket, len(ObjectStorageBucketNames(m)))
	if err != nil {
		return err
	}
	if err := writeBinding(binding, map[string]string{
		"endpoint":          endpoint,
		"bucket":            physicalBucket,
		"region":            "us-east-1",
		"access_key_id":     credentials.AccessKeyID,
		"secret_access_key": credentials.SecretAccessKey,
	}); err != nil {
		return err
	}

	runtimeValues, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	runtimeValues[s3RuntimeKey(logicalBucket, "BUCKET")] = physicalBucket
	runtimeValues[s3RuntimeKey(logicalBucket, "HOST_ENDPOINT")] = endpoint
	runtimeValues[s3RuntimeKey(logicalBucket, "CONTAINER_ENDPOINT")] = "http://seaweedfs:8333"
	if err := writeRuntimeEnv(files.Env, m, runtimeValues); err != nil {
		return err
	}

	values, err := loadApplicationEnvValues(files.ApplicationEnv)
	if err != nil {
		return err
	}
	buckets := ObjectStorageBucketNames(m)
	preferred := preferredServiceInstance(buckets)
	token := envInstanceToken(logicalBucket)
	if logicalBucket == preferred {
		values["S3_ENDPOINT"] = endpoint
		values["S3_BUCKET"] = physicalBucket
		values["S3_REGION"] = "us-east-1"
		values["AWS_ENDPOINT_URL"] = endpoint
		values["AWS_REGION"] = "us-east-1"
		values["AWS_ACCESS_KEY_ID"] = credentials.AccessKeyID
		values["AWS_SECRET_ACCESS_KEY"] = credentials.SecretAccessKey
	}
	if logicalBucket != defaultServiceInstance || len(buckets) != 1 {
		values["S3_"+token+"_ENDPOINT"] = endpoint
		values["S3_"+token+"_BUCKET"] = physicalBucket
		values["S3_"+token+"_REGION"] = "us-east-1"
		values["S3_"+token+"_ACCESS_KEY_ID"] = credentials.AccessKeyID
		values["S3_"+token+"_SECRET_ACCESS_KEY"] = credentials.SecretAccessKey
	}
	if err := writeApplicationEnvValues(files.ApplicationEnv, values); err != nil {
		return err
	}

	return nil
}

func loadApplicationEnvValues(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read application environment contract: %w", err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("application environment contract contains invalid entry")
		}
		values[key] = value
	}
	return values, nil
}

func writeApplicationEnvValues(path string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, values[key])
	}
	if err := writeOwnerOnlyFile(path, []byte(b.String())); err != nil {
		return fmt.Errorf("write application environment contract: %w", err)
	}
	return nil
}
