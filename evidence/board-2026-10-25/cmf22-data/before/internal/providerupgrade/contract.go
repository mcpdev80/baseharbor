package providerupgrade

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Provider string

const (
	ProviderOpenBao  Provider = "openbao"
	ProviderKeycloak Provider = "keycloak"
)

type Classification string

const (
	ClassificationNoChange    Classification = "NO_CHANGE"
	ClassificationSupported   Classification = "SUPPORTED"
	ClassificationUnsupported Classification = "UNSUPPORTED"
)

type ErrorClass string

const (
	ErrorInvalidState    ErrorClass = "INVALID_STATE"
	ErrorUnsupportedPath ErrorClass = "UNSUPPORTED"
	ErrorBackupRequired  ErrorClass = "BACKUP_REQUIRED"
	ErrorBackupInvalid   ErrorClass = "BACKUP_INVALID"
	ErrorApplyFailed     ErrorClass = "APPLY_FAILED"
	ErrorVerifyFailed    ErrorClass = "VERIFY_FAILED"
	ErrorRecoveryFailed  ErrorClass = "RECOVERY_FAILED"
	ErrorDependency      ErrorClass = "DEPENDENCY_FAILED"
)

type Error struct {
	Class ErrorClass
	Op    string
	Err   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Op == "" {
		return string(e.Class) + ": " + e.Err.Error()
	}
	return string(e.Class) + " " + e.Op + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

func Wrap(class ErrorClass, op string, err error) error {
	if err == nil {
		return nil
	}
	var classified *Error
	if errors.As(err, &classified) {
		return err
	}
	return &Error{Class: class, Op: op, Err: err}
}

func ClassOf(err error) ErrorClass {
	var classified *Error
	if errors.As(err, &classified) {
		return classified.Class
	}
	return ""
}

type Inventory struct {
	Provider Provider
	Version  string
	Topology string
	Owner    string
	Healthy  bool
	Details  map[string]string
}

type Request struct {
	CurrentVersion string
	TargetVersion  string
	TargetImage    string
	TargetDigest   string
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.CurrentVersion) == "" || strings.TrimSpace(r.TargetVersion) == "" {
		return errors.New("current and target versions are required")
	}
	if strings.TrimSpace(r.TargetImage) == "" || strings.TrimSpace(r.TargetDigest) == "" {
		return errors.New("target image and immutable digest are required")
	}
	if !strings.HasPrefix(r.TargetDigest, "sha256:") || len(r.TargetDigest) != len("sha256:")+64 {
		return errors.New("target digest must be sha256")
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(r.TargetDigest, "sha256:")); err != nil {
		return errors.New("target digest must contain 64 hexadecimal characters")
	}
	return nil
}

type Assessment struct {
	Classification Classification
	Reason         string
	BackupRequired bool
}

type BackupRef struct {
	Provider  Provider
	ID        string
	Version   string
	CreatedAt time.Time
	Verified  bool
	Metadata  map[string]string
}

func (b BackupRef) Validate(provider Provider, version string) error {
	if b.Provider != provider {
		return fmt.Errorf("backup provider %q does not match %q", b.Provider, provider)
	}
	if strings.TrimSpace(b.ID) == "" {
		return errors.New("backup id is required")
	}
	if b.Version != version {
		return fmt.Errorf("backup version %q does not match installed %q", b.Version, version)
	}
	if !b.Verified {
		return errors.New("backup is not verified")
	}
	return nil
}

type Adapter interface {
	Inventory(context.Context) (Inventory, error)
	Preflight(context.Context, Request) (Assessment, error)
	Backup(context.Context, Request) (BackupRef, error)
	Execute(context.Context, Request, BackupRef) error
	Verify(context.Context, Request) error
	Recover(context.Context, Request, BackupRef) error
}
