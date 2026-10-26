package machine

import (
	"errors"
	"os"
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
