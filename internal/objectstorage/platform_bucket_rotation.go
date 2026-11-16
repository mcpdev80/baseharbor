package objectstorage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/credential"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type PlatformBucketRotationHooks struct {
	Reconcile func(context.Context, PlatformBucket) error
	Verify    func(context.Context, PlatformBucket) error
	Rollback  func(context.Context, PlatformBucket) error
}

type platformBucketRotationState struct {
	Version int                 `json:"version"`
	Old     platformBucketState `json:"old"`
	New     platformBucketState `json:"new"`
}

// RotatePlatformBucketCredentialsAt rotates one provider-owned S3 identity with
// explicit overlap. The replacement identity is created first, consumers are
// reconciled and verified against it, and only then is the previous IAM user
// retired and negatively verified.
func RotatePlatformBucketCredentialsAt(
	ctx context.Context,
	runtime Runtime,
	issuer serviceaccess.Issuer,
	dataDir, namespace, logicalName string,
	hooks PlatformBucketRotationHooks,
) error {
	logicalName = platformBucketToken(logicalName)
	if logicalName == "" {
		return errors.New("platform object-storage bucket name is required")
	}
	if hooks.Reconcile == nil || hooks.Verify == nil {
		return errors.New("platform object-storage credential rotation requires reconcile and verify hooks")
	}

	files, _, _, err := EnsureSharedProviderAt(ctx, runtime, issuer, dataDir, namespace)
	if err != nil {
		return err
	}
	statePath := filepath.Join(files.Dir, "platform-buckets", logicalName+".json")
	rotationPath := filepath.Join(files.Dir, "platform-buckets", logicalName+".rotation.json")
	journal := credential.FileRotationJournal{
		Path: filepath.Join(files.Dir, "platform-bucket-rotation-journal.json"),
	}
	journalKey := "platform-bucket/" + logicalName

	loadOrPrepare := func() (platformBucketRotationState, error) {
		if state, err := loadPlatformBucketRotationState(rotationPath); err == nil {
			return state, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return platformBucketRotationState{}, err
		}
		oldState, err := loadPlatformBucketState(statePath)
		if err != nil {
			return platformBucketRotationState{}, err
		}
		replacement, err := newPlatformBucketState(logicalName)
		if err != nil {
			return platformBucketRotationState{}, err
		}
		replacement.Name = oldState.Name
		replacement.User = rotatedPlatformBucketUser(oldState.User, replacement.AccessKeyID)
		state := platformBucketRotationState{Version: 1, Old: oldState, New: replacement}
		if err := savePlatformBucketRotationState(rotationPath, state); err != nil {
			return platformBucketRotationState{}, err
		}
		return state, nil
	}

	var state platformBucketRotationState
	rotation := credential.Rotation{
		Key:     journalKey,
		Journal: journal,
		Prepare: func(ctx context.Context) error {
			var err error
			state, err = loadOrPrepare()
			if err != nil {
				return err
			}
			return configurePlatformBucketIdentity(ctx, runtime, files, state.New)
		},
		Reconcile: func(ctx context.Context) error {
			var err error
			state, err = loadPlatformBucketRotationState(rotationPath)
			if err != nil {
				return err
			}
			bucket, err := platformBucketFromState(files, state.New)
			if err != nil {
				return err
			}
			return hooks.Reconcile(ctx, bucket)
		},
		Verify: func(ctx context.Context) error {
			var err error
			state, err = loadPlatformBucketRotationState(rotationPath)
			if err != nil {
				return err
			}
			if err := verifyPlatformBucketCredential(ctx, files, state.New, true); err != nil {
				return fmt.Errorf("verify replacement platform S3 credential: %w", err)
			}
			bucket, err := platformBucketFromState(files, state.New)
			if err != nil {
				return err
			}
			return hooks.Verify(ctx, bucket)
		},
		Retire: func(ctx context.Context) error {
			var err error
			state, err = loadPlatformBucketRotationState(rotationPath)
			if err != nil {
				return err
			}
			if _, err := runtime.ExecProjectInput(
				ctx,
				files.Project,
				files.Compose,
				files.Env,
				[]byte(fmt.Sprintf("s3.configure -user=%s -delete -apply\n", state.Old.User)),
				ProviderService,
				"weed",
				"shell",
			); err != nil {
				return fmt.Errorf("retire previous platform S3 identity: %w", err)
			}
			if err := verifyPlatformBucketCredential(ctx, files, state.Old, false); err != nil {
				return err
			}
			if err := savePlatformBucketState(statePath, state.New); err != nil {
				return fmt.Errorf("commit replacement platform S3 identity: %w", err)
			}
			return nil
		},
		Rollback: func(ctx context.Context) error {
			state, err := loadPlatformBucketRotationState(rotationPath)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			_, _ = runtime.ExecProjectInput(
				ctx,
				files.Project,
				files.Compose,
				files.Env,
				[]byte(fmt.Sprintf("s3.configure -user=%s -delete -apply\n", state.New.User)),
				ProviderService,
				"weed",
				"shell",
			)
			if hooks.Rollback == nil {
				return nil
			}
			bucket, err := platformBucketFromState(files, state.Old)
			if err != nil {
				return err
			}
			return hooks.Rollback(ctx, bucket)
		},
	}
	if err := rotation.Run(ctx); err != nil {
		return err
	}
	return os.Remove(rotationPath)
}

func configurePlatformBucketIdentity(ctx context.Context, runtime Runtime, files ProviderFiles, state platformBucketState) error {
	command := fmt.Sprintf(
		"s3.configure -access_key=%s -secret_key=%s -buckets=%s -user=%s -actions=Read,Write,List,Tagging -apply\n",
		state.AccessKeyID,
		state.SecretAccessKey,
		state.Name,
		state.User,
	)
	if _, err := runtime.ExecProjectInput(
		ctx,
		files.Project,
		files.Compose,
		files.Env,
		[]byte(command),
		ProviderService,
		"weed",
		"shell",
	); err != nil {
		return errors.New("configure replacement SeaweedFS platform bucket identity failed")
	}
	return nil
}

func verifyPlatformBucketCredential(ctx context.Context, files ProviderFiles, state platformBucketState, expectValid bool) error {
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return err
	}
	client, err := s3HTTPClient(files)
	if err != nil {
		return err
	}
	credentials := application.ObjectStorageCredentials{
		AccessKeyID:     state.AccessKeyID,
		SecretAccessKey: state.SecretAccessKey,
	}
	status, _, err := signedS3Request(ctx, client, endpoint, http.MethodGet, state.Name, "", credentials, nil)
	if err != nil {
		return err
	}
	if expectValid {
		if status < 200 || status >= 300 {
			return fmt.Errorf("replacement platform S3 credentials returned HTTP %d", status)
		}
		return nil
	}
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		return fmt.Errorf("retired platform S3 credentials remain valid: HTTP %d", status)
	}
	return nil
}

func platformBucketFromState(files ProviderFiles, state platformBucketState) (PlatformBucket, error) {
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

func rotatedPlatformBucketUser(previous, accessKey string) string {
	suffix := strings.ToLower(strings.TrimPrefix(accessKey, "BHPLATFORM"))
	if len(suffix) > 10 {
		suffix = suffix[:10]
	}
	return strings.TrimSpace(previous) + "-r-" + suffix
}

func loadPlatformBucketRotationState(path string) (platformBucketRotationState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return platformBucketRotationState{}, err
	}
	var state platformBucketRotationState
	if err := json.Unmarshal(data, &state); err != nil {
		return platformBucketRotationState{}, err
	}
	if state.Version != 1 ||
		state.Old.Name == "" || state.Old.User == "" || state.Old.AccessKeyID == "" || state.Old.SecretAccessKey == "" ||
		state.New.Name == "" || state.New.User == "" || state.New.AccessKeyID == "" || state.New.SecretAccessKey == "" {
		return platformBucketRotationState{}, errors.New("platform object-storage credential rotation state is incomplete")
	}
	return state, nil
}

func savePlatformBucketRotationState(path string, state platformBucketRotationState) error {
	state.Version = 1
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeCredentialStateFile(path, append(data, '\n'))
}
