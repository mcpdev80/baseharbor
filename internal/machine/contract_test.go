package machine

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/apierror"
)

func TestClassifyExpectedMachinePreconditions(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code ErrorCode
	}{
		{
			name: "os not exist",
			err:  os.ErrNotExist,
			code: ErrorNotFound,
		},
		{
			name: "api not found",
			err:  apierror.New(apierror.CodeNotFound, "missing", 0),
			code: ErrorNotFound,
		},
		{
			name: "api bad request",
			err:  apierror.New(apierror.CodeBadRequest, "bad", 0),
			code: ErrorValidationFailed,
		},
		{
			name: "api conflict",
			err:  apierror.New(apierror.CodeConflict, "conflict", 0),
			code: ErrorConflict,
		},
		{
			name: "unexpected remains internal",
			err:  errors.New("unexpected"),
			code: ErrorInternal,
		},
		{
			name: "config missing classifies to source_missing",
			err:  fmt.Errorf("%w: /root/config/config.yaml", apierror.ErrConfigMissing),
			code: ErrorSourceMissing,
		},
		{
			name: "config empty classifies to source_missing",
			err:  fmt.Errorf("%w: /root/config/config.yaml", apierror.ErrConfigEmpty),
			code: ErrorSourceMissing,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.err)
			if got == nil || got.Code != tc.code {
				t.Fatalf("Classify(%v) = %#v, want code %q", tc.err, got, tc.code)
			}
		})
	}
}

// TestClassifyKeepsConfigMessage ensures the concrete config-error mapping does
// not throw away the human-readable line that LoadConfig already produced: the
// classified message must still contain the original config error text.
func TestClassifyKeepsConfigMessage(t *testing.T) {
	cause := fmt.Errorf("%w: /root/config/config.yaml", apierror.ErrConfigMissing)
	got := Classify(cause)
	if got == nil {
		t.Fatal("Classify(config missing) = nil")
	}
	if got.Code != ErrorSourceMissing {
		t.Fatalf("Classify(config missing) code = %q, want %q", got.Code, ErrorSourceMissing)
	}
	if !strings.Contains(got.Message, "config.yaml is missing") {
		t.Fatalf("Classify(config missing) message = %q, want it to keep the human-readable text", got.Message)
	}
}
