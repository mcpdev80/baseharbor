package openbao

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var ErrManagedCredentialNotFound = errors.New("managed credential does not exist")

const maxManagedCredentialBytes = 1 << 20

// SetManagedCredential stores BaseHarbor-owned/generated credential material in
// the selected managed secret provider. Runtime files remain projections only.
func SetManagedCredential(ctx context.Context, executor Executor, files bhruntime.Files, reference string, value []byte) error {
	reference, err := validateManagedCredentialReference(reference)
	if err != nil { return err }
	if len(value) == 0 { return errors.New("managed credential value must not be empty") }
	if len(value) > maxManagedCredentialBytes { return fmt.Errorf("managed credential value exceeds the %d-byte limit", maxManagedCredentialBytes) }
	if !utf8.Valid(value) { return errors.New("managed credential value must be valid UTF-8 text") }
	credentials, err := LoadAdminCredentials(files)
	if err != nil { return err }
	token, err := loginManager(ctx, executor, files, credentials)
	if err != nil { return err }
	path := "managed/" + reference
	if _, err := execWithTokenInput(ctx, executor, files, token, fmt.Sprintf("exec bao kv put -mount=baseharbor %s value=-", path), value); err != nil {
		return errors.New("write managed credential to OpenBao failed")
	}
	stored, err := readManagedCredentialWithToken(ctx, executor, files, token, path)
	if err != nil { return errors.New("verify managed credential after write failed") }
	if !bytes.Equal(stored, value) { return errors.New("verify managed credential after write failed: stored value differs from stdin payload") }
	return nil
}

// GetManagedCredential returns the authoritative BaseHarbor-managed value.
func GetManagedCredential(ctx context.Context, executor Executor, files bhruntime.Files, reference string) ([]byte, error) {
	reference, err := validateManagedCredentialReference(reference)
	if err != nil { return nil, err }
	credentials, err := LoadAdminCredentials(files)
	if err != nil { return nil, err }
	token, err := loginManager(ctx, executor, files, credentials)
	if err != nil { return nil, err }
	return readManagedCredentialWithToken(ctx, executor, files, token, "managed/"+reference)
}

func readManagedCredentialWithToken(ctx context.Context, executor Executor, files bhruntime.Files, token, path string) ([]byte, error) {
	script := "set +e\nout=\"$(bao kv get -format=json -mount=baseharbor \\\"$1\\\" 2>&1)\"\ncode=$?\nset -e\n" +
		"if [ \\\"$code\\\" -eq 0 ]; then printf \\\"FOUND\\\\n%s\\\" \\\"$out\\\"; exit 0; fi\n" +
		"if printf \\\"%s\\\" \\\"$out\\\" | grep -Fq \\\"No value found\\\"; then printf \\\"NOT_FOUND\\\\n\\\"; exit 0; fi\n" +
		"printf \\\"%s\\\\n\\\" \\\"$out\\\" >&2; exit \\\"$code\\\""
	out, err := execWithToken(ctx, executor, files, token, "sh -ceu "+shellQuote(script)+" -- "+shellQuote(path))
	if err != nil { return nil, errors.New("read managed credential from OpenBao failed") }
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
	if reference == "" { return "", errors.New("managed credential reference is required") }
	if len(reference) > 512 || strings.HasPrefix(reference, "/") || strings.HasSuffix(reference, "/") || strings.Contains(reference, "//") {
		return "", errors.New("managed credential reference is invalid")
	}
	for _, segment := range strings.Split(reference, "/") {
		if segment == "" || segment == "." || segment == ".." { return "", errors.New("managed credential reference is invalid") }
		for _, r := range segment {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
				return "", errors.New("managed credential reference is invalid")
			}
		}
	}
	return reference, nil
}

func shellQuote(value string) string {
	return "\'" + strings.ReplaceAll(value, "\'", "\'\\\"\'\\\"\'") + "\'"
}
