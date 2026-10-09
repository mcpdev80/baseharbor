package coreupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/providerbinding"
	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
)

// BoundProviderRegistry is implemented by providerbinding.Registry. Native
// implementation hooks and ownership checks are constructed by Core; this
// coordinator never creates unverified provider backup or identity fixtures.
type BoundProviderRegistry interface {
	Resolve(context.Context, providerupgrade.Provider) (providerupgrade.Adapter, providerbinding.RuntimeIdentity, error)
	Preflight(context.Context, providerupgrade.Provider, providerupgrade.Request) (providerupgrade.Adapter, providerupgrade.Assessment, error)
}

// BoundProviderTransaction connects the Session 1C adapters to the existing
// ExecuteJournaled lifecycle, including durable per-provider backup references.
type BoundProviderTransaction struct {
	Registry        BoundProviderRegistry
	BackupDirectory string
	RecordExternal  func(context.Context, Delta, string) error
}

func boundProvider(d Delta) (providerupgrade.Provider, providerupgrade.Request, error) {
	var kind providerupgrade.Provider
	switch d.Installed.Kind {
	case Secrets:
		kind = providerupgrade.ProviderOpenBao
	case Identity:
		kind = providerupgrade.ProviderKeycloak
	default:
		return "", providerupgrade.Request{}, errors.New("UNSUPPORTED: no native adapter for this provider kind")
	}
	return kind, providerupgrade.Request{
		CurrentVersion: d.Installed.Version, TargetVersion: d.Desired.Version,
		TargetImage: d.Desired.Image, TargetDigest: d.Desired.Digest,
	}, nil
}

func (t BoundProviderTransaction) backupPath(d Delta) (string, error) {
	if t.BackupDirectory == "" {
		return "", errors.New("durable provider backup receipt directory required")
	}
	h := sha256.Sum256([]byte(JournalKey(d)))
	return filepath.Join(t.BackupDirectory, hex.EncodeToString(h[:])+".backup.json"), nil
}
func (t BoundProviderTransaction) loadBackup(d Delta) (providerupgrade.BackupRef, error) {
	path, err := t.backupPath(d)
	if err != nil {
		return providerupgrade.BackupRef{}, err
	}
	st, err := os.Lstat(path)
	if err != nil {
		return providerupgrade.BackupRef{}, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 65536 {
		return providerupgrade.BackupRef{}, errors.New("unsafe native provider backup receipt")
	}
	f, err := os.Open(path)
	if err != nil {
		return providerupgrade.BackupRef{}, err
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 65537))
	dec.DisallowUnknownFields()
	var backup providerupgrade.BackupRef
	if err := dec.Decode(&backup); err != nil {
		return providerupgrade.BackupRef{}, err
	}
	if dec.Decode(new(any)) != io.EOF {
		return providerupgrade.BackupRef{}, errors.New("provider backup receipt trailing data")
	}
	provider, req, err := boundProvider(d)
	if err != nil {
		return providerupgrade.BackupRef{}, err
	}
	if err := backup.Validate(provider, req.CurrentVersion); err != nil {
		return providerupgrade.BackupRef{}, err
	}
	return backup, nil
}
func (t BoundProviderTransaction) persistBackup(d Delta, backup providerupgrade.BackupRef) error {
	path, err := t.backupPath(d)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("provider backup receipt directory must be private")
	}
	if _, err := os.Lstat(path); err == nil {
		prior, verifyErr := t.loadBackup(d)
		if verifyErr != nil {
			return verifyErr
		}
		if prior.ID != backup.ID || prior.Provider != backup.Provider || prior.Version != backup.Version {
			return errors.New("provider recovery receipt identity changed on retry")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	body, err := json.Marshal(backup)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(body, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

// Hooks provides the only adapter execution integration points to the central
// Core journal. The caller MUST supply a prepared owned registry and private
// journal directory, then use ExecuteJournaled with these hooks.
func (t BoundProviderTransaction) Hooks() Hooks {
	return Hooks{
		Preflight: func(ctx context.Context, p Plan) error {
			if t.Registry == nil || t.RecordExternal == nil || t.BackupDirectory == "" {
				return errors.New("native provider registry, durable receipts and external evidence publisher required")
			}
			for _, d := range p.Deltas {
				if d.Classification == NoChange {
					continue
				}
				provider, req, err := boundProvider(d)
				if err != nil {
					return err
				}
				_, assessment, err := t.Registry.Preflight(ctx, provider, req)
				if err != nil {
					return err
				}
				if assessment.Classification != providerupgrade.ClassificationSupported || !assessment.BackupRequired {
					return fmt.Errorf("UNSUPPORTED: provider %s is not backup-safe for mutation", provider)
				}
			}
			return nil
		},
		RecoveryPoint: func(ctx context.Context, d Delta) error {
			// An interrupted upgrade must reuse its original durable backup.
			// Never take a new snapshot from a possibly partially migrated provider.
			path, pathErr := t.backupPath(d)
			if pathErr != nil {
				return pathErr
			}
			if _, statErr := os.Lstat(path); statErr == nil {
				_, verifyErr := t.loadBackup(d)
				return verifyErr
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
			provider, req, err := boundProvider(d)
			if err != nil {
				return err
			}
			adapter, _, err := t.Registry.Preflight(ctx, provider, req)
			if err != nil {
				return err
			}
			ref, err := adapter.Backup(ctx, req)
			if err != nil {
				return err
			}
			if err := ref.Validate(provider, req.CurrentVersion); err != nil {
				return err
			}
			return t.persistBackup(d, ref)
		},
		Apply: func(ctx context.Context, d Delta) error {
			provider, req, err := boundProvider(d)
			if err != nil {
				return err
			}
			backup, err := t.loadBackup(d)
			if err != nil {
				return err
			}
			adapter, _, err := t.Registry.Resolve(ctx, provider)
			if err != nil {
				return err
			}
			return adapter.Execute(ctx, req, backup)
		},
		Verify: func(ctx context.Context, d Delta) error {
			provider, req, err := boundProvider(d)
			if err != nil {
				return err
			}
			adapter, _, err := t.Registry.Resolve(ctx, provider)
			if err != nil {
				return err
			}
			return adapter.Verify(ctx, req)
		},
		Recover: func(ctx context.Context, d Delta, _ string) error {
			provider, req, err := boundProvider(d)
			if err != nil {
				return err
			}
			backup, err := t.loadBackup(d)
			if err != nil {
				return err
			}
			var adapter providerupgrade.Adapter
			if registry, ok := t.Registry.(interface {
				ResolveRecovery(context.Context, providerupgrade.Provider, providerupgrade.Request) (providerupgrade.Adapter, providerbinding.RuntimeIdentity, error)
			}); ok {
				adapter, _, err = registry.ResolveRecovery(ctx, provider, req)
			} else {
				adapter, _, err = t.Registry.Resolve(ctx, provider)
			}
			if err != nil {
				return err
			}
			return adapter.Recover(ctx, req, backup)
		},
		Record: t.RecordExternal,
	}
}
