package objectstorage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func (d *Driver) waitBucketIdentityReady(ctx context.Context, bucket string, credentials application.ObjectStorageCredentials) error {
	instance, err := d.realization.Existing(ctx)
	if err != nil {
		return err
	}
	client := d.client
	if client == nil {
		client = instance.HTTPClient
	}
	if client == nil {
		return errors.New("SeaweedFS realization did not provide an HTTP client")
	}
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	probeKey := fmt.Sprintf(".baseharbor/identity-ready-%d", time.Now().UnixNano())
	probePayload := []byte("baseharbor-s3-identity-readiness")
	var lastStatus int
	var lastErr error
	for {
		status, _, err := signedS3Request(waitCtx, client, instance.Endpoint, http.MethodPut, bucket, probeKey, credentials, probePayload)
		if err == nil && (status == http.StatusOK || status == http.StatusNoContent) {
			_, _, _ = signedS3Request(context.WithoutCancel(ctx), client, instance.Endpoint, http.MethodDelete, bucket, probeKey, credentials, nil)
			return nil
		}
		lastStatus = status
		lastErr = err
		select {
		case <-waitCtx.Done():
			if lastErr != nil {
				return lastErr
			}
			return fmt.Errorf("S3 identity did not become active before deadline; last HTTP status %d", lastStatus)
		case <-ticker.C:
		}
	}
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
	return signedS3RequestLimit(ctx, client, endpoint, method, bucket, key, credentials, payload, 1<<20)
}

func signedS3RequestLimit(ctx context.Context, client *http.Client, endpoint, method, bucket, key string, credentials application.ObjectStorageCredentials, payload []byte, limit int64) (int, []byte, error) {
	return signedS3RequestQuery(ctx, client, endpoint, method, bucket, key, nil, credentials, payload, limit)
}

func signedS3RequestQuery(ctx context.Context, client *http.Client, endpoint, method, bucket, key string, query url.Values, credentials application.ObjectStorageCredentials, payload []byte, limit int64) (int, []byte, error) {
	path := "/"
	if bucket != "" {
		path += escapePath(bucket)
	}
	if key != "" {
		path += "/" + escapePath(key)
	}
	return signedAWSRequestQuery(ctx, client, endpoint, "s3", method, path, "", query, credentials, payload, limit)
}

func signedAWSRequest(ctx context.Context, client *http.Client, endpoint, service, method, path, contentType string, credentials application.ObjectStorageCredentials, payload []byte) (int, []byte, error) {
	return signedAWSRequestQuery(ctx, client, endpoint, service, method, path, contentType, nil, credentials, payload, 1<<20)
}

func signedAWSRequestQuery(ctx context.Context, client *http.Client, endpoint, service, method, path, contentType string, query url.Values, credentials application.ObjectStorageCredentials, payload []byte, limit int64) (int, []byte, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return 0, nil, err
	}
	if strings.TrimSpace(service) == "" {
		return 0, nil, errors.New("AWS SigV4 service is required")
	}
	if path == "" {
		path = "/"
	}
	if limit < 1 {
		return 0, nil, errors.New("AWS response size limit must be positive")
	}
	base.Path = path
	base.RawQuery = query.Encode()
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	hash := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(hash[:])
	host := base.Host
	canonicalHeaders := "host:" + host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := method + "\n" + base.EscapedPath() + "\n" + base.RawQuery + "\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + payloadHash
	scope := dateStamp + "/us-east-1/" + service + "/aws4_request"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])
	kDate := hmacSHA256([]byte("AWS4"+credentials.SecretAccessKey), dateStamp)
	kRegion := hmacSHA256(kDate, "us-east-1")
	kService := hmacSHA256(kRegion, service)
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
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if int64(len(body)) > limit {
		return resp.StatusCode, nil, fmt.Errorf("AWS response exceeds recovery limit of %d bytes", limit)
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
