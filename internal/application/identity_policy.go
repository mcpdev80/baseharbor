package application

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	IdentityMFAEnv          = "BASEHARBOR_IDENTITY_MFA"
	IdentityMethodsEnv      = "BASEHARBOR_IDENTITY_METHODS"
	IdentityPasswordlessEnv = "BASEHARBOR_IDENTITY_PASSWORDLESS"
)

type EffectiveIdentityPolicy struct {
	MFA          string   `json:"mfa"`
	Methods      []string `json:"methods,omitempty"`
	Passwordless bool     `json:"passwordless"`
	Sources      []string `json:"sources,omitempty"`
}

func ResolveIdentityPolicy(m Manifest) (EffectiveIdentityPolicy, error) {
	if !m.Services.Identity {
		return EffectiveIdentityPolicy{}, nil
	}
	if err := validateIdentityRequirements(m); err != nil {
		return EffectiveIdentityPolicy{}, err
	}

	effective := EffectiveIdentityPolicy{
		MFA:          strings.ToLower(strings.TrimSpace(m.Identity.Authentication.MFA)),
		Methods:      normalizedIdentityMethods(m.Identity.Authentication.Methods),
		Passwordless: m.Identity.Authentication.Passwordless,
		Sources:      []string{"application"},
	}
	if effective.MFA == "" {
		effective.MFA = "optional"
	}

	if raw := strings.ToLower(strings.TrimSpace(os.Getenv(IdentityMFAEnv))); raw != "" {
		switch raw {
		case "disabled", "optional", "required":
		default:
			return EffectiveIdentityPolicy{}, fmt.Errorf("%s must be disabled, optional or required", IdentityMFAEnv)
		}
		if identityMFAStrength(raw) < identityMFAStrength(effective.MFA) {
			return EffectiveIdentityPolicy{}, fmt.Errorf("%s cannot weaken application MFA requirement %q to %q", IdentityMFAEnv, effective.MFA, raw)
		}
		effective.MFA = raw
		effective.Sources = append(effective.Sources, "environment")
	}

	if raw := strings.TrimSpace(os.Getenv(IdentityMethodsEnv)); raw != "" {
		envMethods, err := parseIdentityMethods(raw)
		if err != nil {
			return EffectiveIdentityPolicy{}, fmt.Errorf("%s: %w", IdentityMethodsEnv, err)
		}
		if len(effective.Methods) == 0 {
			effective.Methods = envMethods
		} else {
			allowed := make(map[string]struct{}, len(envMethods))
			for _, method := range envMethods {
				allowed[method] = struct{}{}
			}
			var narrowed []string
			for _, method := range effective.Methods {
				if _, ok := allowed[method]; ok {
					narrowed = append(narrowed, method)
				}
			}
			if len(narrowed) == 0 {
				return EffectiveIdentityPolicy{}, fmt.Errorf("%s removes every application-approved MFA method", IdentityMethodsEnv)
			}
			effective.Methods = narrowed
		}
		effective.Sources = append(effective.Sources, "environment")
	}

	if raw := strings.TrimSpace(os.Getenv(IdentityPasswordlessEnv)); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return EffectiveIdentityPolicy{}, fmt.Errorf("%s must be true or false", IdentityPasswordlessEnv)
		}
		if effective.Passwordless && !value {
			return EffectiveIdentityPolicy{}, fmt.Errorf("%s cannot disable application-required passwordless authentication", IdentityPasswordlessEnv)
		}
		if value {
			effective.Passwordless = true
			effective.Sources = append(effective.Sources, "environment")
		}
	}

	if effective.MFA == "required" && len(effective.Methods) == 0 {
		return EffectiveIdentityPolicy{}, fmt.Errorf("effective identity policy requires MFA but no accepted method is configured")
	}
	if effective.Passwordless && !containsIdentityMethod(effective.Methods, "passkey") && !containsIdentityMethod(effective.Methods, "webauthn") {
		return EffectiveIdentityPolicy{}, fmt.Errorf("effective passwordless policy requires passkey or webauthn")
	}
	effective.Methods = normalizedIdentityMethods(effective.Methods)
	effective.Sources = uniqueIdentityStrings(effective.Sources)
	return effective, nil
}

func identityMFAStrength(value string) int {
	switch value {
	case "required":
		return 2
	case "optional":
		return 1
	default:
		return 0
	}
}

func parseIdentityMethods(raw string) ([]string, error) {
	var values []string
	for _, part := range strings.Split(raw, ",") {
		method := strings.ToLower(strings.TrimSpace(part))
		switch method {
		case "totp", "webauthn", "passkey":
			values = append(values, method)
		case "":
		default:
			return nil, fmt.Errorf("unsupported identity authentication method %q", method)
		}
	}
	values = normalizedIdentityMethods(values)
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one authentication method is required")
	}
	return values, nil
}

func normalizedIdentityMethods(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func containsIdentityMethod(values []string, method string) bool {
	for _, value := range values {
		if value == method {
			return true
		}
	}
	return false
}

func uniqueIdentityStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
