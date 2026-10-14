package identityprovider

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Read only the existing protected binding. An omitted credential/CA input on
// reapply must not split the host, workload and environment projections. A new
// authority cannot inherit another issuer's confidential client credential.
func retainedExternalMaterial(files application.RuntimeFiles, issuer, clientID string, inputs externalDriverInputs) (string, string, error) {
	dir := filepath.Join(files.Bindings, application.IdentityBindingName)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || !bhruntime.ProtectedStateOwner(dir, info) {
		return "", "", errors.New("retained external identity binding owner is unverifiable")
	}
	values := map[string]string{}
	for _, name := range []string{"provider", "oidc.issuer", "oidc.client-id", "client-secret", "ca.crt"} {
		data, err := readExternalProtectedMaterial(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", "", errors.New("retained external identity material is incomplete or unprotected")
		}
		values[name] = strings.TrimSpace(string(data))
		for i := range data {
			data[i] = 0
		}
	}
	sameAuthority := values["provider"] == string(capability.ProviderExternalOIDC) && strings.TrimRight(values["oidc.issuer"], "/") == strings.TrimRight(strings.TrimSpace(issuer), "/") && values["oidc.client-id"] == clientID
	if !sameAuthority {
		// Explicit replacement material is resolved by the caller. Never
		// forward a credential or trust projection to another authority.
		if values["client-secret"] != "" && inputs.secretFile == "" && !inputs.skipRetainedSecret {
			return "", "", errors.New("external identity authority changed; supply its explicit client credential before rebinding; retained credentials were not changed")
		}
		if values["ca.crt"] != "" && inputs.caFile == "" && !inputs.skipRetainedTrust {
			return "", "", errors.New("external identity authority changed; supply its explicit trust material before rebinding; retained trust was not changed")
		}
		return "", "", nil
	}
	if strings.ContainsAny(values["client-secret"], "\r\n") {
		return "", "", errors.New("retained external identity credential is invalid")
	}
	if inputs.skipRetainedSecret {
		values["client-secret"] = ""
	}
	ca := ""
	if values["ca.crt"] != "" && !inputs.skipRetainedTrust {
		ca = filepath.Join(dir, "ca.crt")
	}
	return values["client-secret"], ca, nil
}

func readExternalProtectedMaterial(path string) ([]byte, error) {
	return readExternalMaterial(path, true)
}

func readExternalMaterial(path string, confidential bool) ([]byte, error) {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("external identity material parent is redirected or unavailable")
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0022 != 0 || (confidential && !bhruntime.ProtectedStateOwner(path, before)) || before.Size() > 1<<20 {
		return nil, errors.New("external identity input must be bounded regular material with its required protection")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.New("external identity material changed during inspection")
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	after, statErr := os.Lstat(path)
	if readErr != nil || statErr != nil || !os.SameFile(opened, after) || !after.Mode().IsRegular() || after.Mode().Perm()&0022 != 0 || (confidential && !bhruntime.ProtectedStateOwner(path, after)) || !after.ModTime().Equal(opened.ModTime()) || after.Size() != opened.Size() || len(data) > 1<<20 {
		for i := range data {
			data[i] = 0
		}
		return nil, errors.New("external identity material changed during read")
	}
	return data, nil
}
