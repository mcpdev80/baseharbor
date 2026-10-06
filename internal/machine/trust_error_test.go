package machine

import (
	"github.com/mcpdev80/baseharbor/internal/extension"
	"testing"
)

func TestArtifactTrustErrorsRetainTypedMachineCause(t *testing.T) {
	for _, test := range []struct {
		cause string
		code  ErrorCode
	}{
		{"publisher_denied", ErrorPolicyDenied}, {"artifact_unverifiable", ErrorVerificationFailed},
		{"verification_invalid", ErrorVerificationFailed}, {"trust_metadata_required", ErrorValidationFailed},
	} {
		got := Classify(&extension.TrustError{Code: test.cause, Next: "Use a policy-compliant artifact."})
		if got.Code != test.code || got.CauseCode != test.cause || got.Next == "" {
			t.Fatalf("typed trust cause lost: %#v", got)
		}
	}
}
