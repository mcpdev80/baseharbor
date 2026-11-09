package deployment

import (
	"encoding/json"
	"os"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// testLoadInstallationForOwner is a package-internal test seam (production path
// keeps it nil, so no behaviour changes): it shadows coreinstallation.Load on
// every VerifyDeploymentOwner / VerifyDeploymentOwnerAgainstOwner path. A test
// uses it to deterministically return a synthetic fs error (for example an
// os.ErrPermission) so the access-denied classification can be asserted without
// depending on real file permissions, which are root-dependent on CI runners.
// The nil default preserves byte-identical production behaviour.
var testLoadInstallationForOwner func(root string) (coreinstallation.State, error)

// resolveInstallationLoad shadows the loader with the test seam, falling back to
// the real coreinstallation.Load when the seam is unset. It exists so both the
// production VerifyDeploymentOwner and its loan-bound counterpart resolve the
// same loader through one indirection the test can control.
func resolveInstallationLoad(root string) (coreinstallation.State, error) {
	if testLoadInstallationForOwner != nil {
		return testLoadInstallationForOwner(root)
	}
	return coreinstallation.Load(root)
}

// unmarshalInstallation decodes a Core installation.json into a State.
func unmarshalInstallation(data []byte, state *coreinstallation.State) error {
	return json.Unmarshal(data, state)
}

// marshalInstallation encodes a Core State back to indented JSON bytes.
func marshalInstallation(state coreinstallation.State) ([]byte, error) {
	return json.MarshalIndent(state, "", "  ")
}

// newPermissionDeniedError returns a synthetic fs-style error that satisfies
// errors.Is(err, os.ErrPermission) so the access-denied classification test is
// deterministic and does not depend on real file permissions, which are
// root-dependent on CI runners.
func newPermissionDeniedError() error {
	return &permissionDeniedError{}
}

// permissionDeniedError is a synthetic permission-denied fs error.
type permissionDeniedError struct{}

func (*permissionDeniedError) Error() string { return "permission denied" }
func (*permissionDeniedError) Is(target error) bool {
	return target == os.ErrPermission
}
