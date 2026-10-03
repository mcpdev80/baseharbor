package objectstorage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// PlatformBucket is a provider-owned S3 bucket used by another managed
// BaseHarbor provider (for example Loki or Tempo). It is intentionally separate
// from application object-storage intent and receives a dedicated least-
// privilege IAM identity.
type PlatformBucket struct {
	Name            string
	Endpoint        string
	Network         string
	TrustBundle     string
	AccessKeyID     string
	SecretAccessKey string
}

type platformBucketState struct {
	Version         int    `json:"version"`
	Name            string `json:"name"`
	User            string `json:"user"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

func EnsurePlatformBucketAt(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, dataDir, namespace, logicalName string) (PlatformBucket, error) {
	logicalName = platformBucketToken(logicalName)
	if logicalName == "" {
		return PlatformBucket{}, errors.New("platform object-storage bucket name is required")
	}
	files, _, _, err := EnsureSharedProviderAt(ctx, runtime, issuer, dataDir, namespace)
	if err != nil {
		return PlatformBucket{}, err
	}

	statePath := filepath.Join(files.Dir, "platform-buckets", logicalName+".json")
	state, err := loadPlatformBucketState(statePath)
	if errors.Is(err, os.ErrNotExist) {
		state, err = newPlatformBucketState(logicalName)
		if err != nil {
			return PlatformBucket{}, err
		}
		if err := savePlatformBucketState(statePath, state); err != nil {
			return PlatformBucket{}, err
		}
	} else if err != nil {
		return PlatformBucket{}, err
	}

	list, err := execSeaweedPlatformCommand(ctx, runtime, files, "s3.bucket.list\n")
	if err != nil {
		return PlatformBucket{}, fmt.Errorf("inspect SeaweedFS platform buckets failed: %w", err)
	}
	if !seaweedBucketListed(list, state.Name) {
		command := "s3.bucket.create -name=" + state.Name + "\n"
		if _, err := execSeaweedPlatformCommand(ctx, runtime, files, command); err != nil {
			// The shell command is not a transactional transport boundary. If the
			// process result was lost after SeaweedFS accepted the create, confirm
			// state before reporting failure or issuing another create.
			list, inspectErr := execSeaweedPlatformCommand(ctx, runtime, files, "s3.bucket.list\n")
			if inspectErr != nil || !seaweedBucketListed(list, state.Name) {
				return PlatformBucket{}, fmt.Errorf("create SeaweedFS platform bucket failed: %w", err)
			}
		}
	}
	configure := fmt.Sprintf(
		"s3.configure -access_key=%s -secret_key=%s -buckets=%s -user=%s -actions=Read,Write,List,Tagging -apply\n",
		state.AccessKeyID, state.SecretAccessKey, state.Name, state.User,
	)
	if _, err := execSeaweedPlatformCommand(ctx, runtime, files, configure); err != nil {
		return PlatformBucket{}, fmt.Errorf("configure SeaweedFS platform bucket identity failed: %w", err)
	}
	trust, err := ServiceTrustBundle(files)
	if err != nil {
		return PlatformBucket{}, err
	}
	return PlatformBucket{
		Name:            state.Name,
		Endpoint:        "https://seaweedfs:8443",
		Network:         files.Network,
		TrustBundle:     trust,
		AccessKeyID:     state.AccessKeyID,
		SecretAccessKey: state.SecretAccessKey,
	}, nil
}

func ExistingPlatformBucketAt(dataDir, namespace, logicalName string) (PlatformBucket, error) {
	logicalName = platformBucketToken(logicalName)
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		return PlatformBucket{}, err
	}
	state, err := loadPlatformBucketState(filepath.Join(files.Dir, "platform-buckets", logicalName+".json"))
	if err != nil {
		return PlatformBucket{}, err
	}
	trust, err := ServiceTrustBundle(files)
	if err != nil {
		return PlatformBucket{}, err
	}
	return PlatformBucket{
		Name:            state.Name,
		Endpoint:        "https://seaweedfs:8443",
		Network:         files.Network,
		TrustBundle:     trust,
		AccessKeyID:     state.AccessKeyID,
		SecretAccessKey: state.SecretAccessKey,
	}, nil
}

func newPlatformBucketState(logicalName string) (platformBucketState, error) {
	access, err := randomHex(18)
	if err != nil {
		return platformBucketState{}, err
	}
	secret, err := randomHex(32)
	if err != nil {
		return platformBucketState{}, err
	}
	name := "bh-platform-" + logicalName
	if len(name) > 63 {
		name = name[:63]
	}
	return platformBucketState{
		Version:         1,
		Name:            name,
		User:            "platform-" + logicalName,
		AccessKeyID:     "BHPLATFORM" + strings.ToUpper(access),
		SecretAccessKey: secret,
	}, nil
}

func loadPlatformBucketState(path string) (platformBucketState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return platformBucketState{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return platformBucketState{}, errors.New("platform object-storage credential state is not owner-only")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return platformBucketState{}, err
	}
	var state platformBucketState
	if err := json.Unmarshal(data, &state); err != nil {
		return platformBucketState{}, err
	}
	if state.Version != 1 || state.Name == "" || state.User == "" || state.AccessKeyID == "" || state.SecretAccessKey == "" {
		return platformBucketState{}, errors.New("platform object-storage credential state is incomplete")
	}
	return state, nil
}

func savePlatformBucketState(path string, state platformBucketState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeCredentialStateFile(path, append(data, '\n'))
}

func platformBucketToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func seaweedBucketListed(output, name string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return true
		}
	}
	return false
}

const platformBucketAdminRetryTimeout = 30 * time.Second

func execSeaweedPlatformCommand(ctx context.Context, runtime Runtime, files ProviderFiles, command string) (string, error) {
	retryCtx, cancel := context.WithTimeout(ctx, platformBucketAdminRetryTimeout)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var last error
	for {
		out, err := runtime.ExecProjectInput(retryCtx, files.Project, files.Compose, files.Env, []byte(command), ProviderService, "weed", "shell")
		if err == nil {
			return out, nil
		}
		last = err
		select {
		case <-retryCtx.Done():
			if last == nil {
				last = retryCtx.Err()
			}
			return "", last
		case <-ticker.C:
		}
	}
}
