package objectstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type RuntimeResourceBinding struct {
	ResourceID      string
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}

type RuntimeResourceManager struct {
	dir       string
	endpoint  string
	client    *http.Client
	admin     AdminCredentials
	iam       *IAMClient
}

type runtimeResourceState struct {
	ResourceID      string `json:"resource_id"`
	Application     string `json:"application"`
	Environment     string `json:"environment"`
	Name            string `json:"name"`
	PhysicalBucket  string `json:"physical_bucket"`
	UserName        string `json:"user_name"`
	PolicyName      string `json:"policy_name"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

func NewRuntimeResourceManager(dir, endpoint string, client *http.Client, admin AdminCredentials) (*RuntimeResourceManager, error) {
	dir = strings.TrimSpace(dir)
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if dir == "" || endpoint == "" || client == nil {
		return nil, errors.New("runtime S3 resource manager dependencies are required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create runtime S3 resource state directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("protect runtime S3 resource state directory: %w", err)
	}
	iam, err := NewIAMClient(endpoint, client, admin)
	if err != nil {
		return nil, err
	}
	return &RuntimeResourceManager{dir: dir, endpoint: endpoint, client: client, admin: admin, iam: iam}, nil
}

func (m *RuntimeResourceManager) Create(ctx context.Context, applicationName, environment, name string) (RuntimeResourceBinding, error) {
	state := runtimeResourceIdentity(applicationName, environment, name)
	if existing, err := m.load(state.ResourceID); err == nil {
		return m.binding(existing), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return RuntimeResourceBinding{}, err
	}

	if err := m.iam.CreateUser(ctx, state.UserName); err != nil {
		return RuntimeResourceBinding{}, err
	}
	if err := m.deleteAccessKeys(ctx, state.UserName); err != nil {
		return RuntimeResourceBinding{}, err
	}
	if err := m.ensureBucket(ctx, state.PhysicalBucket); err != nil {
		return RuntimeResourceBinding{}, err
	}
	if err := m.iam.PutUserPolicy(ctx, state.UserName, state.PolicyName, bucketPolicy(state.PhysicalBucket)); err != nil {
		return RuntimeResourceBinding{}, err
	}
	key, err := m.iam.CreateAccessKey(ctx, state.UserName)
	if err != nil {
		return RuntimeResourceBinding{}, err
	}
	state.AccessKeyID = key.AccessKeyID
	state.SecretAccessKey = key.SecretAccessKey
	if err := m.persist(state); err != nil {
		_ = m.iam.DeleteAccessKey(context.WithoutCancel(ctx), state.UserName, key.AccessKeyID)
		return RuntimeResourceBinding{}, err
	}
	return m.binding(state), nil
}

func (m *RuntimeResourceManager) Get(ctx context.Context, applicationName, environment, name string) (RuntimeResourceBinding, error) {
	expected := runtimeResourceIdentity(applicationName, environment, name)
	state, err := m.load(expected.ResourceID)
	if err != nil {
		return RuntimeResourceBinding{}, err
	}
	if state.Application != applicationName || state.Environment != environment || state.Name != name {
		return RuntimeResourceBinding{}, errors.New("runtime S3 resource ownership mismatch")
	}
	status, _, err := signedS3Request(ctx, m.client, m.endpoint, http.MethodHead, state.PhysicalBucket, "", application.ObjectStorageCredentials{
		AccessKeyID: state.AccessKeyID, SecretAccessKey: state.SecretAccessKey,
	}, nil)
	if err != nil {
		return RuntimeResourceBinding{}, err
	}
	if status != http.StatusOK {
		return RuntimeResourceBinding{}, fmt.Errorf("runtime S3 resource verification failed with HTTP %d", status)
	}
	return m.binding(state), nil
}

func (m *RuntimeResourceManager) Delete(ctx context.Context, applicationName, environment, name string) error {
	state := runtimeResourceIdentity(applicationName, environment, name)
	if existing, err := m.load(state.ResourceID); err == nil {
		state = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if state.Application != applicationName || state.Environment != environment || state.Name != name {
		return errors.New("runtime S3 resource ownership mismatch")
	}

	status, _, err := signedS3Request(ctx, m.client, m.endpoint, http.MethodDelete, state.PhysicalBucket, "", application.ObjectStorageCredentials{
		AccessKeyID: m.admin.AccessKeyID, SecretAccessKey: m.admin.SecretAccessKey,
	}, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK && status != http.StatusNotFound {
		return fmt.Errorf("delete runtime S3 bucket failed with HTTP %d", status)
	}
	if err := m.deleteAccessKeys(ctx, state.UserName); err != nil {
		return err
	}
	if err := m.iam.DeleteUserPolicy(ctx, state.UserName, state.PolicyName); err != nil {
		return err
	}
	if err := m.iam.DeleteUser(ctx, state.UserName); err != nil {
		return err
	}
	if err := os.Remove(m.statePath(state.ResourceID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove runtime S3 resource state: %w", err)
	}
	return nil
}

func (m *RuntimeResourceManager) deleteAccessKeys(ctx context.Context, user string) error {
	keys, err := m.iam.ListAccessKeys(ctx, user)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := m.iam.DeleteAccessKey(ctx, user, key); err != nil {
			return err
		}
	}
	return nil
}

func (m *RuntimeResourceManager) ensureBucket(ctx context.Context, bucket string) error {
	credentials := application.ObjectStorageCredentials{AccessKeyID: m.admin.AccessKeyID, SecretAccessKey: m.admin.SecretAccessKey}
	status, _, err := signedS3Request(ctx, m.client, m.endpoint, http.MethodHead, bucket, "", credentials, nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	if status != http.StatusNotFound && status != http.StatusForbidden {
		return fmt.Errorf("inspect runtime S3 bucket failed with HTTP %d", status)
	}
	status, _, err = signedS3Request(ctx, m.client, m.endpoint, http.MethodPut, bucket, "", credentials, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("create runtime S3 bucket failed with HTTP %d", status)
	}
	return nil
}

func (m *RuntimeResourceManager) binding(state runtimeResourceState) RuntimeResourceBinding {
	return RuntimeResourceBinding{
		ResourceID:      state.ResourceID,
		Endpoint:        m.endpoint,
		Bucket:          state.PhysicalBucket,
		Region:          "us-east-1",
		AccessKeyID:     state.AccessKeyID,
		SecretAccessKey: state.SecretAccessKey,
	}
}

func runtimeResourceIdentity(applicationName, environment, name string) runtimeResourceState {
	sum := sha256.Sum256([]byte(applicationName + "\x00" + environment + "\x00" + "object-storage.s3/v1" + "\x00" + name))
	token := hex.EncodeToString(sum[:])
	manifest := application.Manifest{Name: applicationName, Environment: environment}
	return runtimeResourceState{
		ResourceID:     "res-s3-" + token[:24],
		Application:    applicationName,
		Environment:    environment,
		Name:           name,
		PhysicalBucket: PhysicalBucketName(manifest, name),
		UserName:       "bh-runtime-" + token[:24],
		PolicyName:     "bh-runtime-s3-" + token[:16],
	}
}

func bucketPolicy(bucket string) string {
	policy := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Effect":   "Allow",
				"Action":   []string{"s3:ListBucket"},
				"Resource": []string{"arn:aws:s3:::" + bucket},
			},
			{
				"Effect":   "Allow",
				"Action":   []string{"s3:GetObject", "s3:PutObject", "s3:DeleteObject"},
				"Resource": []string{"arn:aws:s3:::" + bucket + "/*"},
			},
		},
	}
	data, _ := json.Marshal(policy)
	return string(data)
}

func (m *RuntimeResourceManager) persist(state runtimeResourceState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := m.statePath(state.ResourceID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write runtime S3 resource state: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit runtime S3 resource state: %w", err)
	}
	return nil
}

func (m *RuntimeResourceManager) load(resourceID string) (runtimeResourceState, error) {
	data, err := os.ReadFile(m.statePath(resourceID))
	if err != nil {
		return runtimeResourceState{}, err
	}
	var state runtimeResourceState
	if err := json.Unmarshal(data, &state); err != nil {
		return runtimeResourceState{}, errors.New("runtime S3 resource state is invalid")
	}
	if state.ResourceID != resourceID || state.Application == "" || state.Environment == "" || state.Name == "" ||
		state.PhysicalBucket == "" || state.UserName == "" || state.PolicyName == "" ||
		state.AccessKeyID == "" || state.SecretAccessKey == "" {
		return runtimeResourceState{}, errors.New("runtime S3 resource state is incomplete")
	}
	return state, nil
}

func (m *RuntimeResourceManager) statePath(resourceID string) string {
	return filepath.Join(m.dir, resourceID+".json")
}
