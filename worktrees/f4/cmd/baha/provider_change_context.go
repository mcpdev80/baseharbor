package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type providerChangeConsentContextKey struct{}
type trustedCLIProviderConsentKey struct{}
type providerConsentValue struct {
	Present bool
	Consent application.ProviderChangeConsent
}

const providerChangeConsentEnv = "BASEHARBOR_PROVIDER_CHANGE_CONSENT"

// Consent is per invocation and retains the existing authorization context.
// It is never persisted or inferred from a generic approval boolean.
func withProviderChangeConsent(ctx context.Context, consent *application.ProviderChangeConsent) context.Context {
	if consent == nil {
		return context.WithValue(ctx, providerChangeConsentContextKey{}, providerConsentValue{})
	}
	return context.WithValue(ctx, providerChangeConsentContextKey{}, providerConsentValue{Present: true, Consent: *consent})
}

func providerChangeConsentFromContext(ctx context.Context) *application.ProviderChangeConsent {
	value, ok := ctx.Value(providerChangeConsentContextKey{}).(providerConsentValue)
	if !ok || !value.Present {
		return nil
	}
	return &value.Consent
}

func invocationProviderChangeConsent(ctx context.Context) (*application.ProviderChangeConsent, error) {
	if _, explicit := ctx.Value(providerChangeConsentContextKey{}).(providerConsentValue); explicit {
		return providerChangeConsentFromContext(ctx), nil
	}
	trusted, _ := ctx.Value(trustedCLIProviderConsentKey{}).(bool)
	if !trusted {
		return nil, nil
	}
	raw := strings.TrimSpace(os.Getenv(providerChangeConsentEnv))
	if raw == "" {
		return nil, nil
	}
	invalid := func() error {
		return &machine.Error{Code: machine.ErrorValidationFailed, CauseCode: "provider_change_consent_input_invalid", Message: "Provider-change consent must be one bounded JSON object with the current plan ID and explicit acknowledgement stages.", Next: "Supply the displayed plan's consent object through BASEHARBOR_PROVIDER_CHANGE_CONSENT; ordinary --yes does not grant this consent."}
	}
	if len(raw) > 8192 || !strings.HasPrefix(raw, "{") {
		return nil, invalid()
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var consent application.ProviderChangeConsent
	if decoder.Decode(&consent) != nil || decoder.Decode(new(any)) != io.EOF || consent.PlanID == "" {
		return nil, invalid()
	}
	return &consent, nil
}

func localCLIProviderConsentContext(ctx context.Context, command []string) context.Context {
	trusted := len(command) > 0 && command[0] != "mcp" && command[0] != "serve"
	return context.WithValue(ctx, trustedCLIProviderConsentKey{}, trusted)
}
