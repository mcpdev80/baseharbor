package objectstorage

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/credential"
)

type bucketCredentialRotationState struct {
	Version int                                  `json:"version"`
	Old     application.ObjectStorageCredentials `json:"old"`
	New     application.ObjectStorageCredentials `json:"new"`
	OldUser string                               `json:"old_user"`
	NewUser string                               `json:"new_user"`
}

type bucketCredentialIdentityState struct {
	Version int    `json:"version"`
	User    string `json:"user"`
}

// RotateBucketCredentials performs overlap-safe S3 credential rotation for one
// logical bucket. The new IAM identity exists before the application binding is
// changed. The old identity is retired only after a real Put/Get verification
// succeeds with the replacement credential pair.
func (d *Driver) RotateBucketCredentials(ctx context.Context, logicalBucket string) error {
	logicalBucket = strings.TrimSpace(logicalBucket)
	if logicalBucket == "" {
		return errors.New("object-storage bucket is required")
	}
	if d.realization == nil {
		return errors.New("SeaweedFS realization is required")
	}
	resource := capability.Resource{
		Application: d.app.Name,
		Kind:        capability.ObjectStorageS3,
		Name:        logicalBucket,
		Provider:    capability.ProviderSeaweedFS,
	}
	physical := PhysicalBucketName(d.app, logicalBucket)
	statePath := bucketCredentialRotationStatePath(d.files, logicalBucket)
	journal := credential.FileRotationJournal{Path: filepath.Join(d.files.Dir, "credential-rotation-journal.json")}
	key := "object-storage.s3/" + logicalBucket

	loadOrPrepare := func() (bucketCredentialRotationState, error) {
		if state, err := loadBucketCredentialRotationState(statePath); err == nil {
			return state, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return bucketCredentialRotationState{}, err
		}
		old, err := application.LoadObjectStorageCredentials(d.files, logicalBucket)
		if err != nil {
			return bucketCredentialRotationState{}, err
		}
		oldUser, err := currentBucketIAMUser(d.files, logicalBucket, physical)
		if err != nil {
			return bucketCredentialRotationState{}, err
		}
		replacement, err := newObjectStorageCredentials()
		if err != nil {
			return bucketCredentialRotationState{}, err
		}
		state := bucketCredentialRotationState{
			Version: 1,
			Old:     old,
			New:     replacement,
			OldUser: oldUser,
			NewUser: rotatedBucketIAMUser(physical, replacement.AccessKeyID),
		}
		if err := saveBucketCredentialRotationState(statePath, state); err != nil {
			return bucketCredentialRotationState{}, err
		}
		return state, nil
	}

	var state bucketCredentialRotationState
	rotation := credential.Rotation{
		Key:     key,
		Journal: journal,
		Prepare: func(ctx context.Context) error {
			var err error
			state, err = loadOrPrepare()
			if err != nil {
				return err
			}
			command := fmt.Sprintf(
				"s3.configure -access_key=%s -secret_key=%s -buckets=%s -user=%s -actions=Read,Write,List,Tagging -apply",
				state.New.AccessKeyID, state.New.SecretAccessKey, physical, state.NewUser,
			)
			return d.runSeaweedShell(ctx, command)
		},
		Reconcile: func(ctx context.Context) error {
			var err error
			state, err = loadBucketCredentialRotationState(statePath)
			if err != nil {
				return err
			}
			if err := application.ReplaceObjectStorageCredentials(d.app, d.files, logicalBucket, state.New); err != nil {
				return err
			}
			return d.Bind(ctx, resource, capability.Binding{})
		},
		Verify: func(ctx context.Context) error {
			state, err := loadBucketCredentialRotationState(statePath)
			if err != nil {
				return err
			}
			current, err := application.LoadObjectStorageCredentials(d.files, logicalBucket)
			if err != nil {
				return err
			}
			if current != state.New {
				return errors.New("object-storage replacement credentials are not the active application binding")
			}
			return d.Verify(ctx, resource, capability.Binding{})
		},
		Retire: func(ctx context.Context) error {
			state, err := loadBucketCredentialRotationState(statePath)
			if err != nil {
				return err
			}
			if err := d.runSeaweedShell(ctx, fmt.Sprintf("s3.configure -user=%s -delete -apply", state.OldUser)); err != nil {
				return fmt.Errorf("retire old SeaweedFS IAM identity: %w", err)
			}
			if err := d.waitBucketCredentialsRetired(ctx, physical, state.Old); err != nil {
				return err
			}
			return saveBucketCredentialIdentityState(d.files, logicalBucket, state.NewUser)
		},
		Rollback: func(ctx context.Context) error {
			state, err := loadBucketCredentialRotationState(statePath)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			_ = d.runSeaweedShell(ctx, fmt.Sprintf("s3.configure -user=%s -delete -apply", state.NewUser))
			if err := application.ReplaceObjectStorageCredentials(d.app, d.files, logicalBucket, state.Old); err != nil {
				return err
			}
			return d.Bind(ctx, resource, capability.Binding{})
		},
	}
	if err := rotation.Run(ctx); err != nil {
		return err
	}
	return os.Remove(statePath)
}

func (d *Driver) waitBucketCredentialsRetired(ctx context.Context, physical string, old application.ObjectStorageCredentials) error {
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	const requiredConsecutiveRejections = 6
	consecutive := 0
	var lastErr error
	for {
		instance, err := d.realization.Existing(waitCtx)
		if err != nil {
			lastErr = err
		} else if instance.HTTPClient == nil {
			lastErr = errors.New("SeaweedFS realization did not provide an HTTP client")
		} else {
			status, _, requestErr := signedS3Request(waitCtx, instance.HTTPClient, instance.Endpoint, http.MethodGet, physical, "", old, nil)
			instance.HTTPClient.CloseIdleConnections()
			switch {
			case requestErr != nil:
				lastErr = requestErr
				consecutive = 0
			case status == http.StatusForbidden || status == http.StatusUnauthorized:
				consecutive++
				if consecutive >= requiredConsecutiveRejections {
					return nil
				}
				lastErr = nil
			default:
				consecutive = 0
				lastErr = fmt.Errorf("old SeaweedFS credentials remain valid after retirement: HTTP %d", status)
			}
		}

		select {
		case <-waitCtx.Done():
			if lastErr == nil {
				lastErr = waitCtx.Err()
			}
			return fmt.Errorf("verify old SeaweedFS credential retirement: %w", lastErr)
		case <-ticker.C:
		}
	}
}

func newObjectStorageCredentials() (application.ObjectStorageCredentials, error) {
	random := func(size int) (string, error) {
		buf := make([]byte, size)
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(buf), nil
	}
	access, err := random(18)
	if err != nil {
		return application.ObjectStorageCredentials{}, err
	}
	secret, err := random(32)
	if err != nil {
		return application.ObjectStorageCredentials{}, err
	}
	return application.ObjectStorageCredentials{AccessKeyID: "BH" + access, SecretAccessKey: secret}, nil
}

func rotatedBucketIAMUser(physical, accessKey string) string {
	suffix := strings.ToLower(strings.TrimPrefix(accessKey, "BH"))
	if len(suffix) > 10 {
		suffix = suffix[:10]
	}
	return physical + "-r-" + suffix
}

func bucketCredentialRotationStatePath(files application.RuntimeFiles, bucket string) string {
	return filepath.Join(files.Dir, "credential-rotation", "object-storage-"+safeCredentialStateToken(bucket)+".json")
}

func bucketCredentialIdentityStatePath(files application.RuntimeFiles, bucket string) string {
	return filepath.Join(files.Dir, "credential-rotation", "object-storage-"+safeCredentialStateToken(bucket)+"-identity.json")
}

func safeCredentialStateToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func loadBucketCredentialRotationState(path string) (bucketCredentialRotationState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return bucketCredentialRotationState{}, err
	}
	var state bucketCredentialRotationState
	if err := json.Unmarshal(data, &state); err != nil {
		return bucketCredentialRotationState{}, err
	}
	if state.Version != 1 || state.Old.AccessKeyID == "" || state.New.AccessKeyID == "" || state.OldUser == "" || state.NewUser == "" {
		return bucketCredentialRotationState{}, errors.New("object-storage credential rotation state is incomplete")
	}
	return state, nil
}

func saveBucketCredentialRotationState(path string, state bucketCredentialRotationState) error {
	state.Version = 1
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeCredentialStateFile(path, append(data, '\n'))
}

func currentBucketIAMUser(files application.RuntimeFiles, bucket, fallback string) (string, error) {
	path := bucketCredentialIdentityStatePath(files, bucket)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	var state bucketCredentialIdentityState
	if err := json.Unmarshal(data, &state); err != nil {
		return "", err
	}
	if state.Version != 1 || strings.TrimSpace(state.User) == "" {
		return "", errors.New("object-storage credential identity state is incomplete")
	}
	return strings.TrimSpace(state.User), nil
}

func saveBucketCredentialIdentityState(files application.RuntimeFiles, bucket, user string) error {
	state := bucketCredentialIdentityState{Version: 1, User: strings.TrimSpace(user)}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeCredentialStateFile(bucketCredentialIdentityStatePath(files, bucket), append(data, '\n'))
}

func writeCredentialStateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
