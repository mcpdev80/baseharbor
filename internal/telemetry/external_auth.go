package telemetry

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func (d *Driver) prepareExternalAuthorization() {
	input, explicit := os.LookupEnv(ExternalHeadersEnv)
	d.externalInput, d.externalExplicit = strings.TrimSpace(input), explicit
	d.externalHeaders, d.externalAuthErr = application.RetainedExternalOTLPAuthorization(d.files, d.externalEndpoint, d.externalInput, explicit)
}

func (d *Driver) revalidateExternalAuthorization() error {
	if d.registeredExternal != nil {
		return d.revalidateRegisteredExternalSource()
	}
	if d.externalAuthErr != nil {
		return d.externalAuthErr
	}
	input, explicit := os.LookupEnv(ExternalHeadersEnv)
	if d.externalEndpoint != application.ExternalOTLPEndpoint() || strings.TrimSpace(input) != d.externalInput || explicit != d.externalExplicit {
		return errors.New("external OTLP endpoint or authorization changed after preparation; replan before delivery")
	}
	current, err := application.RetainedExternalOTLPAuthorization(d.files, d.externalEndpoint, d.externalInput, explicit)
	if err != nil {
		return err
	}
	if current != d.externalHeaders {
		return errors.New("retained external OTLP authorization changed after preparation; replan before delivery")
	}
	return nil
}

// Parse without disclosing header values in diagnostics. The prepared bytes are
// shared by verification and the protected host/workload delivery projections.
func parseExternalHeaders(raw string) (map[string]string, error) {
	result := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return result, nil
	}
	if len(raw) > 1<<20 || strings.ContainsAny(raw, "\r\n\x00") {
		return nil, errors.New("external OTLP authorization headers are invalid")
	}
	for _, item := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(item), "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" || value == "" {
			return nil, errors.New("external OTLP authorization headers are invalid")
		}
		for _, c := range key {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
				return nil, errors.New("external OTLP authorization headers are invalid")
			}
		}
		for _, c := range value {
			if c < 32 || c == 127 {
				return nil, errors.New("external OTLP authorization headers are invalid")
			}
		}
		key = http.CanonicalHeaderKey(key)
		if _, exists := result[key]; exists {
			return nil, errors.New("external OTLP authorization headers are duplicated")
		}
		result[key] = value
	}
	return result, nil
}
