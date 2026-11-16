package providerbinding

import (
	"context"
	"errors"

	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
)

// Failure is the central journal-facing classification of a provider adapter
// failure. The caller must not infer safe retry solely from network errors.
type Failure struct {
	Code             string
	Retryable        bool
	RecoveryRequired bool
}

func Classify(err error, mutated bool) Failure {
	if err == nil {
		return Failure{Code: "OK"}
	}
	if mutated {
		return Failure{Code: "RECOVERY_REQUIRED", RecoveryRequired: true}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return Failure{Code: "INTERRUPTED"}
	}
	switch providerupgrade.ClassOf(err) {
	case providerupgrade.ErrorUnsupportedPath:
		return Failure{Code: "UNSUPPORTED"}
	case providerupgrade.ErrorBackupRequired:
		return Failure{Code: "BACKUP_UNAVAILABLE"}
	case providerupgrade.ErrorBackupInvalid:
		return Failure{Code: "BACKUP_INVALID"}
	case providerupgrade.ErrorDependency:
		return Failure{Code: "DEPENDENCY_FAILED"}
	case providerupgrade.ErrorApplyFailed:
		return Failure{Code: "RECOVERY_REQUIRED", RecoveryRequired: true}
	case providerupgrade.ErrorRecoveryFailed:
		return Failure{Code: "RECOVERY_REQUIRED", RecoveryRequired: true}
	case providerupgrade.ErrorVerifyFailed:
		return Failure{Code: "VERIFICATION_FAILED"}
	case providerupgrade.ErrorInvalidState:
		return Failure{Code: "INVALID_STATE"}
	default:
		return Failure{Code: "INVALID_STATE"}
	}
}
