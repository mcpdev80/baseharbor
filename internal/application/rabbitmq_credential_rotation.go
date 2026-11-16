package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/credential"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type rabbitMQCredentialRotationMaterial struct {
	OldAppUser     string `json:"old_app_user"`
	OldAppPassword string `json:"old_app_password"`
	NewAppUser     string `json:"new_app_user"`
	NewAppPassword string `json:"new_app_password"`
	OldAdminUser   string `json:"old_admin_user,omitempty"`
	NewAdminUser   string `json:"new_admin_user,omitempty"`
	OldAdminPass   string `json:"old_admin_password,omitempty"`
	NewAdminPass   string `json:"new_admin_password,omitempty"`
}

func RotateRabbitMQCredential(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, instance string) error {
	if runtime == nil {
		return errors.New("RabbitMQ credential rotation requires a runtime provider")
	}
	found := false
	for _, name := range RabbitMQInstanceNames(m) {
		if name == instance {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("RabbitMQ instance %q is not declared", instance)
	}

	key := "rabbitmq/" + instance + "/credentials"
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
			var material rabbitMQCredentialRotationMaterial
			if len(data) == 0 {
				values, err := readRuntimeEnv(files.Env)
				if err != nil {
					return err
				}
				oldUser, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "USER"))
				if err != nil {
					return err
				}
				oldPassword, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "PASSWORD"))
				if err != nil {
					return err
				}
				token, err := randomApplicationSecret(6)
				if err != nil {
					return err
				}
				newPassword, err := randomApplicationSecret(32)
				if err != nil {
					return err
				}
				material = rabbitMQCredentialRotationMaterial{
					OldAppUser:     oldUser,
					OldAppPassword: oldPassword,
					NewAppUser:     oldUser + "_r_" + strings.ReplaceAll(token, "-", "_"),
					NewAppPassword: newPassword,
				}
				if m.Services.MessagingManagementUI {
					material.OldAdminUser, err = requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "ADMIN_USER"))
					if err != nil {
						return err
					}
					material.OldAdminPass, err = requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "ADMIN_PASSWORD"))
					if err != nil {
						return err
					}
					material.NewAdminUser = material.OldAdminUser + "_r_" + strings.ReplaceAll(token, "-", "_")
					material.NewAdminPass, err = randomApplicationSecret(32)
					if err != nil {
						return err
					}
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
			service := rabbitmqMemberServiceName(instance, 0)
			if err := rabbitMQCreateApplicationUser(ctx, runtime, files, service, material.NewAppUser, material.NewAppPassword); err != nil {
				return fmt.Errorf("prepare RabbitMQ application credential: %w", err)
			}
			if material.NewAdminUser != "" {
				if err := rabbitMQCreateAdminUser(ctx, runtime, files, service, material.NewAdminUser, material.NewAdminPass); err != nil {
					return fmt.Errorf("prepare RabbitMQ management credential: %w", err)
				}
			}
			return nil
		},
		Reconcile: func(ctx context.Context) error {
			material, err := loadPreparedRabbitMQCredential(prepared, key)
			if err != nil {
				return err
			}
			values, err := readRuntimeEnv(files.Env)
			if err != nil {
				return err
			}
			values[rabbitmqRuntimeKey(instance, "USER")] = material.NewAppUser
			values[rabbitmqRuntimeKey(instance, "PASSWORD")] = material.NewAppPassword
			if material.NewAdminUser != "" {
				values[rabbitmqRuntimeKey(instance, "ADMIN_USER")] = material.NewAdminUser
				values[rabbitmqRuntimeKey(instance, "ADMIN_PASSWORD")] = material.NewAdminPass
			}
			if err := writeRuntimeEnv(files.Env, m, values); err != nil {
				return err
			}
			_, err = EnsureRuntimeContract(m, files)
			return err
		},
		Verify: func(ctx context.Context) error {
			material, err := loadPreparedRabbitMQCredential(prepared, key)
			if err != nil {
				return err
			}
			service := rabbitmqMemberServiceName(instance, 0)
			if err := rabbitMQVerifyCredential(ctx, runtime, files, service, material.NewAppUser, material.NewAppPassword, true); err != nil {
				return err
			}
			if material.NewAdminUser != "" {
				if err := rabbitMQVerifyCredential(ctx, runtime, files, service, material.NewAdminUser, material.NewAdminPass, true); err != nil {
					return err
				}
			}
			if err := VerifyRabbitMQRuntime(ctx, m, files); err != nil {
				return err
			}
			if err := VerifyRabbitMQHACluster(ctx, runtimeCredentialExecutor{runtime: runtime, files: files}, m, files); err != nil {
				return err
			}
			if m.Services.MessagingManagementUI {
				return VerifyApplicationManagementUIs(ctx, m, files)
			}
			return nil
		},
		Retire: func(ctx context.Context) error {
			material, err := loadPreparedRabbitMQCredential(prepared, key)
			if err != nil {
				return err
			}
			service := rabbitmqMemberServiceName(instance, 0)
			if err := rabbitMQDeleteUser(ctx, runtime, files, service, material.OldAppUser); err != nil {
				return fmt.Errorf("retire previous RabbitMQ application credential: %w", err)
			}
			if err := rabbitMQVerifyCredential(ctx, runtime, files, service, material.OldAppUser, material.OldAppPassword, false); err != nil {
				return err
			}
			if material.NewAdminUser != "" {
				if err := rabbitMQDeleteUser(ctx, runtime, files, service, material.OldAdminUser); err != nil {
					return fmt.Errorf("retire previous RabbitMQ management credential: %w", err)
				}
				if err := rabbitMQVerifyCredential(ctx, runtime, files, service, material.OldAdminUser, material.OldAdminPass, false); err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(ctx context.Context) error {
			material, err := loadPreparedRabbitMQCredential(prepared, key)
			if err != nil {
				return err
			}
			service := rabbitmqMemberServiceName(instance, 0)
			values, err := readRuntimeEnv(files.Env)
			if err != nil {
				return err
			}
			values[rabbitmqRuntimeKey(instance, "USER")] = material.OldAppUser
			values[rabbitmqRuntimeKey(instance, "PASSWORD")] = material.OldAppPassword
			if material.NewAdminUser != "" {
				values[rabbitmqRuntimeKey(instance, "ADMIN_USER")] = material.OldAdminUser
				values[rabbitmqRuntimeKey(instance, "ADMIN_PASSWORD")] = material.OldAdminPass
			}
			if err := writeRuntimeEnv(files.Env, m, values); err != nil {
				return err
			}
			if _, err := EnsureRuntimeContract(m, files); err != nil {
				return err
			}
			_ = rabbitMQDeleteUser(ctx, runtime, files, service, material.NewAppUser)
			if material.NewAdminUser != "" {
				_ = rabbitMQDeleteUser(ctx, runtime, files, service, material.NewAdminUser)
			}
			return nil
		},
	}).Run(ctx)
}
