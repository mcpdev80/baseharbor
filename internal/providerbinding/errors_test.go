package providerbinding

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
)

func TestClassifyRecoveryIsNeverAutoRetry(t *testing.T) {
	cases := []struct {
		err     error
		mutated bool
		code    string
		recover bool
	}{
		{providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "compat", errors.New("unsupported")), false, "UNSUPPORTED", false},
		{providerupgrade.Wrap(providerupgrade.ErrorBackupInvalid, "snapshot", errors.New("invalid")), false, "BACKUP_INVALID", false},
		{providerupgrade.Wrap(providerupgrade.ErrorDependency, "sql", errors.New("missing")), false, "DEPENDENCY_FAILED", false},
		{providerupgrade.Wrap(providerupgrade.ErrorVerifyFailed, "oidc", errors.New("invalid issuer")), false, "VERIFICATION_FAILED", false},
		{providerupgrade.Wrap(providerupgrade.ErrorApplyFailed, "apply", errors.New("interrupted")), false, "RECOVERY_REQUIRED", true},
		{context.Canceled, false, "INTERRUPTED", false},
		{errors.New("unknown"), true, "RECOVERY_REQUIRED", true},
	}
	for _, tc := range cases {
		got := Classify(tc.err, tc.mutated)
		if got.Code != tc.code || got.RecoveryRequired != tc.recover || got.Retryable {
			t.Fatalf("unsafe classification %+v for %v", got, tc.err)
		}
	}
}
