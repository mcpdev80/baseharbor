package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/credential"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type mongoDBCredentialRotationMaterial struct {
	Database         string `json:"database"`
	OldAppUser       string `json:"old_app_user"`
	OldAppPassword   string `json:"old_app_password"`
	NewAppUser       string `json:"new_app_user"`
	NewAppPassword   string `json:"new_app_password"`
	OldAdminUser     string `json:"old_admin_user"`
	OldAdminPassword string `json:"old_admin_password"`
	NewAdminUser     string `json:"new_admin_user"`
	NewAdminPassword string `json:"new_admin_password"`
}

func RotateMongoDBCredential(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, instance string) error {
	if runtime == nil {
		return errors.New("MongoDB credential rotation requires a runtime provider")
	}
	found := false
	for _, name := range DocumentDatabaseInstanceNames(m) {
		if name == instance {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("MongoDB instance %q is not declared", instance)
	}

	key := "mongodb/" + instance + "/credentials"
	journal := credential.FileRotationJournal{Path: filepath.Join(files.Dir, "credential-rotation.json")}
	prepared := credential.FilePreparedMaterialStore{Dir: filepath.Join(files.Dir, "credential-rotation-prepared")}

	return (credential.Rotation{
		Key:      key,
		Journal:  journal,
		Prepared: prepared,
		Prepare: func(ctx context.Context) error {
			data, err := prepared.Load(key)
			if err != nil {
				return err
			}
			var material mongoDBCredentialRotationMaterial
			if len(data) == 0 {
				values, err := readRuntimeEnv(files.Env)
				if err != nil {
					return err
				}
				material.Database, err = requireRuntimeValue(values, mongodbRuntimeKey(instance, "DB"))
				if err != nil {
					return err
				}
				material.OldAppUser, err = requireRuntimeValue(values, mongodbRuntimeKey(instance, "USER"))
				if err != nil {
					return err
				}
				material.OldAppPassword, err = requireRuntimeValue(values, mongodbRuntimeKey(instance, "PASSWORD"))
				if err != nil {
					return err
				}
				material.OldAdminUser, err = requireRuntimeValue(values, mongodbRuntimeKey(instance, "ADMIN_USER"))
				if err != nil {
					return err
				}
				material.OldAdminPassword, err = requireRuntimeValue(values, mongodbRuntimeKey(instance, "ADMIN_PASSWORD"))
				if err != nil {
					return err
				}
				token, err := randomApplicationSecret(6)
				if err != nil {
					return err
				}
				token = strings.ReplaceAll(token, "-", "_")
				material.NewAppUser = material.OldAppUser + "_r_" + token
				material.NewAdminUser = material.OldAdminUser + "_r_" + token
				material.NewAppPassword, err = randomApplicationSecret(32)
				if err != nil {
					return err
				}
				material.NewAdminPassword, err = randomApplicationSecret(32)
				if err != nil {
					return err
				}
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

			authority, err := mongoDBCredentialAuthority(ctx, runtime, m, files, instance)
			if err != nil {
				return err
			}
			if err := mongoDBUpsertUser(ctx, runtime, files, authority, material.OldAdminUser, material.OldAdminPassword, material.Database, material.NewAppUser, material.NewAppPassword, "readWrite"); err != nil {
				return fmt.Errorf("prepare MongoDB application credential: %w", err)
			}
			if err := mongoDBUpsertUser(ctx, runtime, files, authority, material.OldAdminUser, material.OldAdminPassword, "admin", material.NewAdminUser, material.NewAdminPassword, "root"); err != nil {
				return fmt.Errorf("prepare MongoDB administration credential: %w", err)
			}
			return nil
		},
		Reconcile: func(ctx context.Context) error {
			material, err := loadPreparedMongoDBCredential(prepared, key)
			if err != nil {
				return err
			}
			values, err := readRuntimeEnv(files.Env)
			if err != nil {
				return err
			}
			values[mongodbRuntimeKey(instance, "USER")] = material.NewAppUser
			values[mongodbRuntimeKey(instance, "PASSWORD")] = material.NewAppPassword
			values[mongodbRuntimeKey(instance, "ADMIN_USER")] = material.NewAdminUser
			values[mongodbRuntimeKey(instance, "ADMIN_PASSWORD")] = material.NewAdminPassword
			if err := writeRuntimeEnv(files.Env, m, values); err != nil {
				return err
			}
			if _, err := EnsureRuntimeContract(m, files); err != nil {
				return err
			}
			return rollMongoDBCredentialConsumers(ctx, runtime, m, files, instance)
		},
		Verify: func(ctx context.Context) error {
			material, err := loadPreparedMongoDBCredential(prepared, key)
			if err != nil {
				return err
			}
			authority, err := mongoDBCredentialAuthority(ctx, runtime, m, files, instance)
			if err != nil {
				return err
			}
			if err := mongoDBVerifyCredential(ctx, runtime, files, authority, material.NewAppUser, material.NewAppPassword, material.Database, true); err != nil {
				return err
			}
			if err := mongoDBVerifyCredential(ctx, runtime, files, authority, material.NewAdminUser, material.NewAdminPassword, "admin", true); err != nil {
				return err
			}
			if err := VerifyMongoDBRuntime(ctx, m, files); err != nil {
				return err
			}
			if err := VerifyMongoDBHACluster(ctx, runtimeCredentialExecutor{runtime: runtime, files: files}, m, files); err != nil {
				return err
			}
			if m.Services.DocumentDatabaseManagementUI {
				return VerifyApplicationManagementUIs(ctx, m, files)
			}
			return nil
		},
		Retire: func(ctx context.Context) error {
			material, err := loadPreparedMongoDBCredential(prepared, key)
			if err != nil {
				return err
			}
			authority, err := mongoDBCredentialAuthority(ctx, runtime, m, files, instance)
			if err != nil {
				return err
			}
			if err := mongoDBDropUser(ctx, runtime, files, authority, material.NewAdminUser, material.NewAdminPassword, material.Database, material.OldAppUser); err != nil {
				return err
			}
			if err := mongoDBDropUser(ctx, runtime, files, authority, material.NewAdminUser, material.NewAdminPassword, "admin", material.OldAdminUser); err != nil {
				return err
			}
			if err := mongoDBVerifyCredential(ctx, runtime, files, authority, material.OldAppUser, material.OldAppPassword, material.Database, false); err != nil {
				return err
			}
			return mongoDBVerifyCredential(ctx, runtime, files, authority, material.OldAdminUser, material.OldAdminPassword, "admin", false)
		},
		Rollback: func(ctx context.Context) error {
			material, err := loadPreparedMongoDBCredential(prepared, key)
			if err != nil {
				return err
			}
			values, err := readRuntimeEnv(files.Env)
			if err != nil {
				return err
			}
			values[mongodbRuntimeKey(instance, "USER")] = material.OldAppUser
			values[mongodbRuntimeKey(instance, "PASSWORD")] = material.OldAppPassword
			values[mongodbRuntimeKey(instance, "ADMIN_USER")] = material.OldAdminUser
			values[mongodbRuntimeKey(instance, "ADMIN_PASSWORD")] = material.OldAdminPassword
			if err := writeRuntimeEnv(files.Env, m, values); err != nil {
				return err
			}
			if _, err := EnsureRuntimeContract(m, files); err != nil {
				return err
			}
			if err := rollMongoDBCredentialConsumers(ctx, runtime, m, files, instance); err != nil {
				return err
			}
			authority, err := mongoDBCredentialAuthority(ctx, runtime, m, files, instance)
			if err != nil {
				return err
			}
			_ = mongoDBDropUser(ctx, runtime, files, authority, material.OldAdminUser, material.OldAdminPassword, material.Database, material.NewAppUser)
			_ = mongoDBDropUser(ctx, runtime, files, authority, material.OldAdminUser, material.OldAdminPassword, "admin", material.NewAdminUser)
			return nil
		},
	}).Run(ctx)
}

func mongoDBCredentialAuthority(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, instance string) (string, error) {
	if mongodbMemberCount(m, instance) <= 1 {
		return mongodbMemberServiceName(instance, 0), nil
	}
	return MongoDBHAPrimary(ctx, runtimeCredentialExecutor{runtime: runtime, files: files}, m, instance)
}

func rollMongoDBCredentialConsumers(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, instance string) error {
	environment, err := RuntimeEnvironment(files)
	if err != nil {
		return err
	}
	primary, err := mongoDBCredentialAuthority(ctx, runtime, m, files, instance)
	if err != nil {
		return err
	}
	var order []string
	for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
		service := mongodbMemberServiceName(instance, ordinal)
		if service != primary {
			order = append(order, service)
		}
	}
	order = append(order, primary)
	op := runtimeCredentialExecutor{runtime: runtime, files: files}
	for _, service := range order {
		if err := runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, files.Dir, environment, []string{service}, files.Compose); err != nil {
			return fmt.Errorf("recreate MongoDB credential consumer %s: %w", service, err)
		}
		if mongodbMemberCount(m, instance) > 1 {
			deadline := time.Now().Add(90 * time.Second)
			for {
				if err := VerifyMongoDBHACluster(ctx, op, m, files); err == nil {
					break
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("MongoDB HA did not recover after credential projection to %s", service)
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
				}
			}
		}
	}
	if m.Services.DocumentDatabaseManagementUI {
		return runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, files.Dir, environment, []string{mongodbUIServiceName(instance)}, files.Compose)
	}
	return nil
}
