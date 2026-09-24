package serviceaccess

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LifecycleObservation is the secret-safe, provider-neutral certificate
// lifecycle state exposed to status/doctor/evidence surfaces.
type LifecycleObservation struct {
	Source          PKISource `json:"source"`
	LifecycleOwner  string    `json:"lifecycle_owner"`
	RenewalMode     string    `json:"renewal_mode"`
	IssuerReference string    `json:"issuer_reference,omitempty"`
	ServerExpiresAt time.Time `json:"server_expires_at"`
	ClientExpiresAt time.Time `json:"client_expires_at,omitempty"`
	Health          string    `json:"health"`
	Warning         string    `json:"warning,omitempty"`
}

// InspectLifecycle reads already-materialized lifecycle state. It never issues,
// renews, revokes or otherwise mutates certificate material.
func InspectLifecycle(dir string) (LifecycleObservation, error) {
	managedPath := filepath.Join(dir, "state.json")
	if data, err := os.ReadFile(managedPath); err == nil {
		var state managedPKIState
		if err := json.Unmarshal(data, &state); err != nil {
			return LifecycleObservation{}, fmt.Errorf("decode managed service PKI lifecycle: %w", err)
		}
		if state.Version != 2 {
			return LifecycleObservation{}, fmt.Errorf("unsupported managed PKI state version %d", state.Version)
		}
		source := state.Source
		if source == "" {
			source = PKIManagedLocal
		}
		health, warning := lifecycleHealth(state.ServerExpiresAt, state.RenewalMode)
		return LifecycleObservation{
			Source:          source,
			LifecycleOwner:  state.LifecycleOwner,
			RenewalMode:     state.RenewalMode,
			IssuerReference: state.IssuerReference,
			ServerExpiresAt: state.ServerExpiresAt,
			ClientExpiresAt: state.ClientExpiresAt,
			Health:          health,
			Warning:         warning,
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return LifecycleObservation{}, err
	}

	staticPath := filepath.Join(dir, "static-state.json")
	data, err := os.ReadFile(staticPath)
	if err != nil {
		return LifecycleObservation{}, err
	}
	var state staticPKIState
	if err := json.Unmarshal(data, &state); err != nil {
		return LifecycleObservation{}, fmt.Errorf("decode static service PKI lifecycle: %w", err)
	}
	if state.Version != 1 {
		return LifecycleObservation{}, fmt.Errorf("unsupported static PKI state version %d", state.Version)
	}
	health, warning := lifecycleHealth(state.ServerExpiresAt, state.RenewalMode)
	if strings.TrimSpace(state.Warning) != "" {
		warning = state.Warning
	}
	if strings.TrimSpace(state.Health) != "" {
		health = state.Health
	}
	return LifecycleObservation{
		Source:          state.Source,
		LifecycleOwner:  state.LifecycleOwner,
		RenewalMode:     state.RenewalMode,
		ServerExpiresAt: state.ServerExpiresAt,
		ClientExpiresAt: state.ClientExpiresAt,
		Health:          health,
		Warning:         warning,
	}, nil
}

func lifecycleHealth(expiresAt time.Time, renewalMode string) (string, string) {
	if expiresAt.IsZero() {
		return "unknown", "certificate expiry is not recorded"
	}
	remaining := time.Until(expiresAt)
	switch {
	case remaining <= 0:
		return "critical", "certificate is expired"
	case remaining <= managedCertificateRenewalWindow:
		if renewalMode == "automatic-reconcile" {
			return "critical", "certificate is inside the renewal window but has not been rotated yet; reconcile the service now"
		}
		return "critical", "certificate expires within 7 days; replacement is required"
	case remaining <= 30*24*time.Hour:
		if renewalMode == "automatic-reconcile" {
			return "warn", "certificate expires within 30 days; the next service reconcile will renew it before the 7-day renewal window"
		}
		return "warn", "certificate expires within 30 days; prepare replacement material"
	default:
		return "ok", ""
	}
}
