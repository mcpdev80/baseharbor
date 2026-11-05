package openbao

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var ErrManagedCredentialNotFound = errors.New("managed credential does not exist")

// ErrManagedCredentialAlreadyExists documents a create-only (compare-and-swap)
// seed that could not run because a managed object already exists for the
// reference. The fresh-device reconcile path treats this as the winning case:
// an existing managed authority keeps its value and is read back, so a
// concurrently created managed value is never overwritten by the stale local
// candidate. It is aliased from the low-level deployment package so the
// devaccess binder can detect the same sentinel without an import cycle.
var ErrManagedCredentialAlreadyExists = deployment.ErrManagedCredentialAlreadyExists

const maxManagedCredentialBytes = 1 << 20

// SetManagedCredential stores BaseHarbor-owned/generated credential material in
// the selected managed secret provider. Runtime files remain projections only.
func SetManagedCredential(ctx context.Context, executor Executor, files bhruntime.Files, reference string, value []byte) error {
	reference, err := validateManagedCredentialReference(reference)
	if err != nil {
		return err
	}
	if len(value) == 0 {
		return errors.New("managed credential value must not be empty")
	}
	if len(value) > maxManagedCredentialBytes {
		return fmt.Errorf("managed credential value exceeds the %d-byte limit", maxManagedCredentialBytes)
	}
	if !utf8.Valid(value) {
		return errors.New("managed credential value must be valid UTF-8 text")
	}

	credentials, err := LoadAdminCredentials(files)
	if err != nil {
		return err
	}
	token, err := loginManager(ctx, executor, files, credentials)
	if err != nil {
		return err
	}

	path := "managed/" + reference
	if _, err := execWithTokenInput(ctx, executor, files, token, fmt.Sprintf("exec bao kv put -mount=baseharbor %s value=-", path), value); err != nil {
		return errors.New("write managed credential to OpenBao failed")
	}
	stored, err := readManagedCredentialWithToken(ctx, executor, files, token, path)
	if err != nil {
		return errors.New("verify managed credential after write failed")
	}
	if !bytes.Equal(stored, value) {
		return errors.New("verify managed credential after write failed: stored value differs from stdin payload")
	}
	return nil
}

// SetManagedCredentialIfAbsent seeds the managed credential only when no
// managed object already exists for reference. It is the create-only /
// compare-and-swap variant of SetManagedCredential used by the fresh-device
// reconcile path: a concurrent managed value that appeared after the initial
// not-found lookup keeps its value, so it is never overwritten with the stale
// local candidate.
//
// It returns ErrManagedCredentialAlreadyExists when the object already exists
// and leaves that value untouched; any other returned error is a genuine write
// or infrastructure failure the caller must surface rather than racing an
// unconditional write.
func SetManagedCredentialIfAbsent(ctx context.Context, executor Executor, files bhruntime.Files, reference string, value []byte) error {
	reference, err := validateManagedCredentialReference(reference)
	if err != nil {
		return err
	}
	if len(value) == 0 {
		return errors.New("managed credential value must not be empty")
	}
	if len(value) > maxManagedCredentialBytes {
		return fmt.Errorf("managed credential value exceeds the %d-byte limit", maxManagedCredentialBytes)
	}
	if !utf8.Valid(value) {
		return errors.New("managed credential value must be valid UTF-8 text")
	}

	credentials, err := LoadAdminCredentials(files)
	if err != nil {
		return err
	}
	token, err := loginManager(ctx, executor, files, credentials)
	if err != nil {
		return err
	}

	path := "managed/" + reference
	// Create-only / compare-and-swap write: seed only when the reference does
	// not already exist. -if-not-exists makes the existence check and the write
	// atomic on the existing baseharbor KV v2 mount, so a concurrent managed
	// value can never be clobbered by the stale local candidate. On a
	// pre-existing reference the write fails closed and the caller returns the
	// already-managed value.
	if _, err := execWithTokenInput(ctx, executor, files, token, fmt.Sprintf("exec bao kv put -if-not-exists -mount=baseharbor %s value=-", path), value); err != nil {
		return ErrManagedCredentialAlreadyExists
	}
	return nil
}

// GetManagedCredential returns the authoritative BaseHarbor-managed value.
func GetManagedCredential(ctx context.Context, executor Executor, files bhruntime.Files, reference string) ([]byte, error) {
	reference, err := validateManagedCredentialReference(reference)
	if err != nil {
		return nil, err
	}
	credentials, err := LoadAdminCredentials(files)
	if err != nil {
		return nil, err
	}
	token, err := loginManager(ctx, executor, files, credentials)
	if err != nil {
		return nil, err
	}
	return readManagedCredentialWithToken(ctx, executor, files, token, "managed/"+reference)
}

func readManagedCredentialWithToken(ctx context.Context, executor Executor, files bhruntime.Files, token, path string) ([]byte, error) {
	const script = `IFS= read -r BAO_TOKEN
export BAO_TOKEN
set +e
out="$(bao kv get -format=json -mount=baseharbor "$1" 2>&1)"
code=$?
set -e
if [ "$code" -eq 0 ]; then
  printf 'FOUND\n%s' "$out"
  exit 0
fi
if printf '%s' "$out" | grep -Fq 'No value found'; then
  printf 'NOT_FOUND\n'
  exit 0
fi
printf '%s\n' "$out" >&2
exit "$code"`

	input := []byte(token + "\n")
	out, err := executor.ExecProjectInput(
		ctx,
		projectNameForFiles(files),
		files.Compose,
		files.Env,
		input,
		serviceName,
		"sh",
		"-ceu",
		script,
		"--",
		path,
	)
	if err != nil {
		return nil, errors.New("read managed credential from OpenBao failed")
	}

	switch {
	case strings.HasPrefix(out, "NOT_FOUND\n"):
		return nil, ErrManagedCredentialNotFound
	case strings.HasPrefix(out, "FOUND\n"):
		return decodeApplicationSecretValue(strings.TrimPrefix(out, "FOUND\n"))
	default:
		return nil, errors.New("read managed credential from OpenBao returned an invalid response")
	}
}

func validateManagedCredentialReference(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", errors.New("managed credential reference is required")
	}
	if len(reference) > 512 || strings.HasPrefix(reference, "/") || strings.HasSuffix(reference, "/") || strings.Contains(reference, "//") {
		return "", errors.New("managed credential reference is invalid")
	}
	for _, segment := range strings.Split(reference, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("managed credential reference is invalid")
		}
		for _, r := range segment {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
				return "", errors.New("managed credential reference is invalid")
			}
		}
	}
	return reference, nil
}
