package objectstorage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	ProviderProject = "baseharbor-object-storage"
	ProviderService = "seaweedfs"
	ProviderNetwork = "baseharbor-object-storage"
	ProviderImage   = "chrislusf/seaweedfs:4.47"
)

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	ExecProject(context.Context, string, string, string, string, ...string) (string, error)
	DestroyProject(context.Context, string, string, string) error
}

type Driver struct {
	runtime        Runtime
	app            application.Manifest
	files          application.RuntimeFiles
	client         *http.Client
	createdBuckets map[string]struct{}
}

type ProviderFiles struct {
	Dir     string
	Compose string
	Env     string
}

func NewDriver(runtime Runtime, app application.Manifest, files application.RuntimeFiles) *Driver {
	return &Driver{runtime: runtime, app: app, files: files, client: &http.Client{Timeout: 10 * time.Second}, createdBuckets: map[string]struct{}{}}
}

func (d *Driver) Descriptor() capability.Provider { return capability.SeaweedFS }

func (d *Driver) Preflight(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.ObjectStorageS3 || resource.Provider != capability.ProviderSeaweedFS {
		return fmt.Errorf("SeaweedFS provider cannot satisfy %s via %s", resource.Kind, resource.Provider)
	}
	if binding.ObjectStorageS3 == nil || strings.TrimSpace(binding.ObjectStorageS3.Bucket) == "" {
		return errors.New("S3 bucket binding is required")
	}
	if binding.Security == nil {
		return errors.New("S3 secure binding is required")
	}
	if err := binding.Security.Validate(); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Provision(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	providerFiles, err := EnsureProviderFiles()
	if err != nil {
		return err
	}
	if err := d.runtime.ConfigProject(ctx, ProviderProject, providerFiles.Compose, providerFiles.Env); err != nil {
		return fmt.Errorf("validate SeaweedFS provider configuration: %w", err)
	}
	if err := d.runtime.UpProject(ctx, ProviderProject, providerFiles.Compose, providerFiles.Env); err != nil {
		return fmt.Errorf("start SeaweedFS provider: %w", err)
	}
	endpoint, err := providerEndpoint(providerFiles)
	if err != nil {
		return err
	}
	if err := waitS3(ctx, d.client, endpoint); err != nil {
		return fmt.Errorf("wait for SeaweedFS S3 readiness: %w", err)
	}

	credentials, err := application.LoadObjectStorageCredentials(d.files, resource.Name)
	if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, resource.Name)
	configure := fmt.Sprintf("s3.configure -access_key=%s -secret_key=%s -buckets=%s -user=%s -actions=Read,Write,List,Tagging -apply",
		credentials.AccessKeyID, credentials.SecretAccessKey, physical, physical)
	if _, err := d.runtime.ExecProject(ctx, ProviderProject, providerFiles.Compose, providerFiles.Env, ProviderService, "weed", "shell", "-command="+configure); err != nil {
		return fmt.Errorf("configure least-privilege S3 identity for %s: %w", resource.Name, err)
	}

	status, _, err := signedS3Request(ctx, d.client, endpoint, http.MethodHead, physical, "", credentials, nil)
	if err != nil {
		return fmt.Errorf("inspect S3 bucket %s: %w", resource.Name, err)
	}
	if status >= 200 && status < 300 {
		return nil
	}
	if status != http.StatusNotFound {
		return fmt.Errorf("inspect S3 bucket %s: unexpected HTTP status %d", resource.Name, status)
	}
	create := fmt.Sprintf("s3.bucket.create -name=%s -owner=%s", physical, physical)
	if _, err := d.runtime.ExecProject(ctx, ProviderProject, providerFiles.Compose, providerFiles.Env, ProviderService, "weed", "shell", "-command="+create); err != nil {
		return fmt.Errorf("create S3 bucket %s: %w", resource.Name, err)
	}
	d.createdBuckets[resource.Name] = struct{}{}
	return nil
}

func (d *Driver) Bind(_ context.Context, resource capability.Resource, _ capability.Binding) error {
	providerFiles, err := ExistingProviderFiles()
	if err != nil {
		return err
	}
	endpoint, err := providerEndpoint(providerFiles)
	if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, resource.Name)
	if err := application.MaterializeObjectStorageBinding(d.app, d.files, resource.Name, physical, endpoint); err != nil {
		return fmt.Errorf("materialize S3 application binding: %w", err)
	}
	return nil
}

func (d *Driver) Verify(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	providerFiles, err := ExistingProviderFiles()
	if err != nil {
		return err
	}
	credentials, err := application.LoadObjectStorageCredentials(d.files, resource.Name)
	if err != nil {
		return err
	}
	endpoint, err := providerEndpoint(providerFiles)
	if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, resource.Name)
	var probe [18]byte
	if _, err := rand.Read(probe[:]); err != nil {
		return fmt.Errorf("generate S3 verification probe: %w", err)
	}
	key := ".baseharbor/verify-" + hex.EncodeToString(probe[:6])
	payload := []byte("baseharbor-s3-verification-" + hex.EncodeToString(probe[:]))

	status, _, err := signedS3Request(ctx, d.client, endpoint, http.MethodPut, physical, key, credentials, payload)
	if err != nil {
		return fmt.Errorf("S3 PutObject verification: %w", err)
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("S3 PutObject verification returned HTTP %d", status)
	}
	defer signedS3Request(context.WithoutCancel(ctx), d.client, endpoint, http.MethodDelete, physical, key, credentials, nil)

	status, body, err := signedS3Request(ctx, d.client, endpoint, http.MethodGet, physical, key, credentials, nil)
	if err != nil {
		return fmt.Errorf("S3 GetObject verification: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("S3 GetObject verification returned HTTP %d", status)
	}
	if !bytes.Equal(body, payload) {
		return errors.New("S3 GetObject verification returned unexpected payload")
	}
	return nil
}

func (d *Driver) Rollback(ctx context.Context) {
	for bucket := range d.createdBuckets {
		_ = d.DestroyBucket(ctx, bucket)
	}
}

func (d *Driver) DestroyBucket(ctx context.Context, logicalBucket string) error {
	providerFiles, err := ExistingProviderFiles()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	physical := PhysicalBucketName(d.app, logicalBucket)
	command := fmt.Sprintf("s3.bucket.delete -name=%s", physical)
	if _, err := d.runtime.ExecProject(ctx, ProviderProject, providerFiles.Compose, providerFiles.Env, ProviderService, "weed", "shell", "-command="+command); err != nil {
		return fmt.Errorf("destroy S3 bucket %s: %w", logicalBucket, err)
	}
	revoke := fmt.Sprintf("s3.configure -user=%s -delete -apply", physical)
	if _, err := d.runtime.ExecProject(ctx, ProviderProject, providerFiles.Compose, providerFiles.Env, ProviderService, "weed", "shell", "-command="+revoke); err != nil {
		return fmt.Errorf("revoke S3 identity for %s: %w", logicalBucket, err)
	}
	return nil
}

func PhysicalBucketName(m application.Manifest, logical string) string {
	base := "bh-" + m.Name + "-" + m.Environment + "-" + logical
	if len(base) <= 63 {
		return base
	}
	sum := sha256.Sum256([]byte(base))
	suffix := "-" + hex.EncodeToString(sum[:])[:10]
	return strings.TrimRight(base[:63-len(suffix)], "-") + suffix
}

func EnsureProviderFiles() (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	dir := filepath.Join(dataDir, "providers", "seaweedfs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ProviderFiles{}, fmt.Errorf("create SeaweedFS provider state: %w", err)
	}
	files := ProviderFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env")}
	values := map[string]string{}
	if data, err := os.ReadFile(files.Env); err == nil {
		values, err = parseEnv(data)
		if err != nil {
			return ProviderFiles{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProviderFiles{}, err
	}
	if values["BASEHARBOR_SEAWEEDFS_PORT"] == "" {
		port, err := allocatePort()
		if err != nil {
			return ProviderFiles{}, err
		}
		values["BASEHARBOR_SEAWEEDFS_PORT"] = strconv.Itoa(port)
	}
	if err := writeEnv(files.Env, values); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAML()), 0o600); err != nil {
		return ProviderFiles{}, fmt.Errorf("write SeaweedFS provider compose: %w", err)
	}
	if err := os.Chmod(files.Compose, 0o600); err != nil {
		return ProviderFiles{}, err
	}
	return files, nil
}

func ExistingProviderFiles() (ProviderFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return ProviderFiles{}, err
	}
	dir := filepath.Join(dataDir, "providers", "seaweedfs")
	files := ProviderFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env")}
	for _, path := range []string{files.Compose, files.Env} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroySharedProvider(ctx context.Context, runtime Runtime) error {
	files, err := ExistingProviderFiles()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, ProviderProject, files.Compose, files.Env); err != nil {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func providerComposeYAML() string {
	return `services:
  seaweedfs:
    image: chrislusf/seaweedfs:4.47
    restart: unless-stopped
    command: server -s3 -iam=true
    ports:
      - "127.0.0.1:${BASEHARBOR_SEAWEEDFS_PORT}:8333"
    volumes:
      - seaweedfs-data:/data
    networks:
      object-storage:
        aliases:
          - seaweedfs

volumes:
  seaweedfs-data:

networks:
  object-storage:
    name: baseharbor-object-storage
`
}

func providerEndpoint(files ProviderFiles) (string, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return "", err
	}
	values, err := parseEnv(data)
	if err != nil {
		return "", err
	}
	port := values["BASEHARBOR_SEAWEEDFS_PORT"]
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid SeaweedFS provider port")
	}
	return "http://127.0.0.1:" + port, nil
}

func waitS3(ctx context.Context, client *http.Client, endpoint string) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			if last == nil {
				last = ctx.Err()
			}
			return last
		case <-ticker.C:
		}
	}
}

func signedS3Request(ctx context.Context, client *http.Client, endpoint, method, bucket, key string, credentials application.ObjectStorageCredentials, payload []byte) (int, []byte, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return 0, nil, err
	}
	path := "/"
	if bucket != "" {
		path += escapePath(bucket)
	}
	if key != "" {
		path += "/" + escapePath(key)
	}
	base.Path = path
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	hash := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(hash[:])
	host := base.Host
	canonicalHeaders := "host:" + host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := method + "\n" + base.EscapedPath() + "\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + payloadHash
	scope := dateStamp + "/us-east-1/s3/aws4_request"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])
	kDate := hmacSHA256([]byte("AWS4"+credentials.SecretAccessKey), dateStamp)
	kRegion := hmacSHA256(kDate, "us-east-1")
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
	authorization := "AWS4-HMAC-SHA256 Credential=" + credentials.AccessKeyID + "/" + scope + ", SignedHeaders=" + signedHeaders + ", Signature=" + signature

	req, err := http.NewRequestWithContext(ctx, method, base.String(), bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("Authorization", authorization)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func hmacSHA256(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

func escapePath(value string) string {
	parts := strings.Split(value, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func parseEnv(data []byte) (map[string]string, error) {
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return nil, errors.New("invalid SeaweedFS provider environment")
		}
		values[key] = value
	}
	return values, nil
}

func writeEnv(path string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, values[key])
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func allocatePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
