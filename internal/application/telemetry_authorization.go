package application

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func readWorkloadRuntimeEnv(m Manifest, path string) (map[string]string, error) {
	if HasOTLPTelemetry(m) {
		return readProtectedOTLPRuntimeEnv(path)
	}
	return readRuntimeEnv(path)
}

// Reapply can reuse this deployment's authorization at the same authority. A
// changed authority receives only explicitly selected material, including an
// explicitly empty input for a destination that needs no authorization.
func RetainedExternalOTLPAuthorization(files RuntimeFiles, endpoint, input string, explicit bool) (string, error) {
	values, err := readProtectedOTLPRuntimeEnv(files.Env)
	if errors.Is(err, os.ErrNotExist) {
		appValues, appErr := readProtectedOTLPRuntimeEnv(files.ApplicationEnv)
		if appErr != nil && !errors.Is(appErr, os.ErrNotExist) {
			return "", appErr
		}
		if appValues["OTEL_EXPORTER_OTLP_ENDPOINT"] != "" {
			return "", errors.New("retained OTLP delivery has no protected runtime owner; recover it before rebinding")
		}
		return input, nil
	}
	if err != nil {
		return "", err
	}
	if explicit || values["OTLP_PROVIDER"] != string(capability.ProviderExternalOTLP) {
		return input, nil
	}
	if strings.TrimRight(values["OTLP_HOST_ENDPOINT"], "/") != strings.TrimRight(endpoint, "/") {
		if values["OTLP_HEADERS"] != "" {
			return "", errors.New("external OTLP authority changed; select its authorization explicitly; retained authorization was not forwarded")
		}
		return input, nil
	}
	return values["OTLP_HEADERS"], nil
}

func readProtectedOTLPRuntimeEnv(path string) (map[string]string, error) {
	if err := checkPrivateRuntimeOwnerParents(path); err != nil {
		return nil, err
	}
	before, err := existingPrivateRuntimeOwner(path)
	if err != nil {
		return nil, err
	}
	if before == nil {
		return nil, os.ErrNotExist
	}
	if before.Mode().Perm()&0077 != 0 || before.Size() > 1<<20 {
		return nil, errors.New("OTLP deployment authorization owner is unprotected or oversized")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("OTLP deployment authorization owner is unavailable")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.New("OTLP deployment authorization owner changed")
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	defer func() {
		for i := range data {
			data[i] = 0
		}
	}()
	after, statErr := existingPrivateRuntimeOwner(path)
	if readErr != nil || statErr != nil || after == nil || !os.SameFile(opened, after) || after.Mode().Perm()&0077 != 0 || after.Size() != opened.Size() || !after.ModTime().Equal(opened.ModTime()) || len(data) > 1<<20 {
		return nil, errors.New("OTLP deployment authorization owner changed during read")
	}
	return readRuntimeEnvBytes(data)
}

// VerifyOTLPHostDelivery compares the protected host projection with the actual
// runtime owner, without exposing confidential values through errors.
func VerifyOTLPHostDelivery(m Manifest, files RuntimeFiles) error {
	values, err := readProtectedOTLPRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	appEnv, err := readProtectedOTLPRuntimeEnv(files.ApplicationEnv)
	if err != nil {
		return err
	}
	for runtimeKey, hostKey := range map[string]string{"OTLP_HOST_ENDPOINT": "OTEL_EXPORTER_OTLP_ENDPOINT", "OTLP_HEADERS": "OTEL_EXPORTER_OTLP_HEADERS", OTLPTLSHostCAEnv: "OTEL_EXPORTER_OTLP_CERTIFICATE", OTLPTLSHostClientCertEnv: "OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", OTLPTLSHostClientKeyEnv: "OTEL_EXPORTER_OTLP_CLIENT_KEY"} {
		if appEnv[hostKey] != values[runtimeKey] {
			return errors.New("OTLP host projection differs from protected runtime delivery")
		}
	}
	return nil
}
