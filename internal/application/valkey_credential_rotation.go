package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/credential"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type valkeyCredentialRotationMaterial struct {
	Old string `json:"old"`
	New string `json:"new"`
}

func RotateValkeyCredential(ctx context.Context, runtime bhruntime.RuntimeProvider, issuer serviceaccess.Issuer, m Manifest, files RuntimeFiles, instance string) error {
	if runtime == nil {
		return errors.New("Valkey credential rotation requires a runtime provider")
	}
	found := false
	for _, name := range ValkeyInstanceNames(m) {
		if name == instance {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("Valkey instance %q is not declared", instance)
	}

	key := "valkey/" + instance + "/password"
	journal := credential.FileRotationJournal{Path: filepath.Join(files.Dir, "credential-rotation.json")}
	prepared := credential.FilePreparedMaterialStore{Dir: filepath.Join(files.Dir, "credential-rotation-prepared")}

	load := func() (valkeyCredentialRotationMaterial, error) {
		data, err := prepared.Load(key)
		if err != nil {
			return valkeyCredentialRotationMaterial{}, err
		}
		if len(data) == 0 {
			return valkeyCredentialRotationMaterial{}, errors.New("prepared Valkey credential material is missing")
		}
		var material valkeyCredentialRotationMaterial
		if err := json.Unmarshal(data, &material); err != nil {
			return valkeyCredentialRotationMaterial{}, fmt.Errorf("decode prepared Valkey credential material: %w", err)
		}
		if material.Old == "" || material.New == "" || material.Old == material.New {
			return valkeyCredentialRotationMaterial{}, errors.New("prepared Valkey credential material is invalid")
		}
		return material, nil
	}

	return (credential.Rotation{
		Key:      key,
		Journal:  journal,
		Prepared: prepared,
		Prepare: func(ctx context.Context) error {
			data, err := prepared.Load(key)
			if err != nil {
				return err
			}
			var material valkeyCredentialRotationMaterial
			if len(data) == 0 {
				values, err := readRuntimeEnv(files.Env)
				if err != nil {
					return err
				}
				oldPassword, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
				if err != nil {
					return err
				}
				newPassword, err := randomApplicationSecret(32)
				if err != nil {
					return err
				}
				material = valkeyCredentialRotationMaterial{Old: oldPassword, New: newPassword}
				encoded, err := json.Marshal(material)
				if err != nil {
					return err
				}
				if err := prepared.Save(key, encoded); err != nil {
					return err
				}
			} else if err := json.Unmarshal(data, &material); err != nil {
				return err
			}
			for ordinal := 0; ordinal < valkeyMemberCount(m, instance); ordinal++ {
				service := valkeyMemberServiceName(instance, ordinal)
				if err := valkeyAddPassword(ctx, runtime, files, service, material.Old, material.New); err != nil {
					return fmt.Errorf("prepare Valkey credential on %s: %w", service, err)
				}
			}
			return nil
		},
		Reconcile: func(ctx context.Context) error {
			material, err := load()
			if err != nil {
				return err
			}
			return valkeyReconcileCredentialConsumers(ctx, runtime, issuer, m, files, instance, material.Old, material.New)
		},
		Verify: func(ctx context.Context) error {
			material, err := load()
			if err != nil {
				return err
			}
			for ordinal := 0; ordinal < valkeyMemberCount(m, instance); ordinal++ {
				service := valkeyMemberServiceName(instance, ordinal)
				if err := valkeyVerifyPassword(ctx, runtime, files, service, material.New, true); err != nil {
					return fmt.Errorf("verify rotated Valkey credential on %s: %w", service, err)
				}
			}
			deadline := time.Now().Add(30 * time.Second)
			for {
				if err := VerifyValkeyRuntime(ctx, runtime, m, files); err == nil {
					break
				} else if time.Now().After(deadline) {
					return err
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
				}
			}
			op := runtimeCredentialExecutor{runtime: runtime, files: files}
			if err := VerifyValkeyHACluster(ctx, op, m, files); err != nil {
				return err
			}
			if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
				return VerifyApplicationManagementUIs(ctx, m, files)
			}
			return nil
		},
		Retire: func(ctx context.Context) error {
			material, err := load()
			if err != nil {
				return err
			}
			for ordinal := 0; ordinal < valkeyMemberCount(m, instance); ordinal++ {
				service := valkeyMemberServiceName(instance, ordinal)
				if err := valkeyRemovePassword(ctx, runtime, files, service, material.New, material.Old); err != nil {
					return fmt.Errorf("retire previous Valkey credential on %s: %w", service, err)
				}
			}
			for ordinal := 0; ordinal < valkeyMemberCount(m, instance); ordinal++ {
				service := valkeyMemberServiceName(instance, ordinal)
				if err := valkeyVerifyPassword(ctx, runtime, files, service, material.Old, false); err != nil {
					return fmt.Errorf("verify previous Valkey credential rejection on %s: %w", service, err)
				}
			}
			return nil
		},
		Rollback: func(ctx context.Context) error {
			material, err := load()
			if err != nil {
				return err
			}
			return valkeyRollbackCredentialConsumers(ctx, runtime, issuer, m, files, instance, material.Old, material.New)
		},
	}).Run(ctx)
}

type runtimeCredentialExecutor struct {
	runtime bhruntime.RuntimeProvider
	files   RuntimeFiles
}

func (e runtimeCredentialExecutor) Run(ctx context.Context, service string, args ...string) (string, error) {
	return e.runtime.ExecProject(ctx, e.files.Project, e.files.Compose, e.files.Env, service, args...)
}

func (e runtimeCredentialExecutor) RunSensitive(ctx context.Context, service string, input []byte, args ...string) (string, error) {
	return e.runtime.ExecProjectInput(ctx, e.files.Project, e.files.Compose, e.files.Env, input, service, args...)
}
