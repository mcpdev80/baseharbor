package application

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Empty optional material is not consent to drop an existing confidential
// client or trust authority. Provider adapters must retain the original material
// or supply an explicitly selected replacement before changing any projection.
func checkIdentityMaterialContinuation(files RuntimeFiles, clientSecret string, trust []byte) error {
	for name, supplied := range map[string]bool{"client-secret": clientSecret != "", "ca.crt": len(trust) != 0} {
		if supplied {
			continue
		}
		for _, root := range []string{files.Bindings, workloadServiceBindingProjectionDir(files)} {
			_, err := os.Lstat(filepath.Join(root, IdentityBindingName, name))
			if err == nil {
				return fmt.Errorf("existing identity %s requires retained or explicitly replaced material; no credential or trust downgrade was applied", name)
			}
			if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("existing identity material inventory is unverifiable")
			}
		}
	}
	return nil
}

// Binding consistency is distinct from login, token/refresh validity and future
// rotation. Verify all existing delivery projections instead of merely checking
// that a host file is nonempty. No secret value belongs in an error/result.
func verifyIdentityCredentialProjections(files RuntimeFiles) error {
	host := filepath.Join(files.Bindings, IdentityBindingName)
	workload := filepath.Join(workloadServiceBindingProjectionDir(files), IdentityBindingName)
	runtimeValues, err := readRuntimeEnv(files.Env)
	if err != nil {
		return fmt.Errorf("identity runtime credential projection is unavailable")
	}
	appValues, err := loadApplicationEnvValues(files.ApplicationEnv)
	if err != nil {
		return fmt.Errorf("identity application credential projection is unavailable")
	}
	for _, name := range []string{"provider", "oidc.client-id", "client-secret", "ca.crt"} {
		a, aErr := os.ReadFile(filepath.Join(host, name))
		b, bErr := os.ReadFile(filepath.Join(workload, name))
		optional := name == "client-secret" || name == "ca.crt"
		if optional && errors.Is(aErr, os.ErrNotExist) && errors.Is(bErr, os.ErrNotExist) {
			if name == "client-secret" && (runtimeValues["IDENTITY_CLIENT_SECRET"] != "" || appValues["OIDC_CLIENT_SECRET_FILE"] != "") || name == "ca.crt" && (runtimeValues["IDENTITY_CA_FILE"] != "" || appValues["OIDC_CA_FILE"] != "") {
				return fmt.Errorf("identity optional material has stale environment references")
			}
			continue
		}
		if aErr != nil || bErr != nil || !bytes.Equal(a, b) {
			return fmt.Errorf("identity host and workload %s projections differ or are unavailable", name)
		}
		switch name {
		case "oidc.client-id":
			if strings.TrimSpace(string(a)) != runtimeValues["IDENTITY_CLIENT_ID"] || runtimeValues["IDENTITY_CLIENT_ID"] != appValues["OIDC_CLIENT_ID"] {
				return fmt.Errorf("identity client-id environment projections differ")
			}
		case "client-secret":
			if strings.TrimSpace(string(a)) == "" || strings.TrimSpace(string(a)) != runtimeValues["IDENTITY_CLIENT_SECRET"] || appValues["OIDC_CLIENT_SECRET_FILE"] != filepath.Join(host, name) {
				return fmt.Errorf("identity confidential credential environment projections differ")
			}
		case "ca.crt":
			if len(bytes.TrimSpace(a)) == 0 || runtimeValues["IDENTITY_CA_FILE"] != filepath.Join(host, name) || appValues["OIDC_CA_FILE"] != filepath.Join(host, name) {
				return fmt.Errorf("identity trust environment projections differ")
			}
		}
	}
	return nil
}
