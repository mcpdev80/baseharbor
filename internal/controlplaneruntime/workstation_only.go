package controlplaneruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// registerWorkstationOnlyAPI exposes the same Core machine operations to an
// enrolled workstation without inventing an OIDC operator. The only anonymous
// endpoint consumes a previously minted local-owner one-use invitation.
// Every remaining API operation passes the existing per-request device
// registry/CA/CRL middleware and is restricted to its tenant and editor role.
func registerWorkstationOnlyAPI(ctx context.Context, mux *http.ServeMux, cfg Config,
	deps serverDependencies, machineExecutor machinehttp.Executor) error {
	if !cfg.WorkstationEnrollmentEnabled || cfg.ConnectorEnrollmentEnabled ||
		cfg.operatorAPIEnabled() || deps.pool == nil || machineExecutor == nil {
		return errors.New("device-only API requires an owned workstation Core and database")
	}
	provider, ok := machineExecutor.(workstationEnrollmentProvider)
	if !ok {
		return errors.New("Core is missing workstation enrollment authority")
	}
	handler, err := provider.WorkstationEnrollmentHTTP(ctx,
		database.NewWorkstationEnrollmentStore(deps.pool), deps.pool,
		cfg.ConnectorAuthorityTarget)
	if err != nil {
		return err
	}
	delegated, ok := handler.(interface {
		ServeDelegatedEnroll(http.ResponseWriter, *http.Request)
		ServeDeviceRenew(http.ResponseWriter, *http.Request)
	})
	if !ok {
		return errors.New("workstation authority cannot enroll and renew devices")
	}
	mux.HandleFunc("POST /api/v1/workstations/enroll", delegated.ServeDelegatedEnroll)

	identity, ok := machineExecutor.(interface {
		ManagementInstallation(context.Context) (string, string, error)
	})
	if !ok {
		return errors.New("Core is missing the owned installation identity")
	}
	if strings.TrimSpace(cfg.RuntimeOperationsDir) == "" {
		return ErrMissingOperationsDir
	}
	machineHandler, err := machinehttp.New(machineExecutor, cfg.RuntimeOperationsDir)
	if err != nil {
		return err
	}
	deviceMux := http.NewServeMux()
	deviceMux.Handle("/api/v1/machine/", machineHandler)
	deviceMux.HandleFunc("POST /api/v1/workstations/renew", delegated.ServeDeviceRenew)
	deviceMux.HandleFunc("GET /api/v1/management/installation", func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := tenancy.FromContext(r.Context())
		if !ok || tenant.TenantID == "" || !strings.HasPrefix(tenant.ExternalIdentityID, "workstation/") {
			http.Error(w, "registered workstation required", http.StatusForbidden)
			return
		}
		id, issuer, err := identity.ManagementInstallation(r.Context())
		if err != nil || id == "" || issuer == "" {
			http.Error(w, "Core installation identity unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(struct {
			Version        string `json:"version"`
			InstallationID string `json:"installation_id"`
			IdentityIssuer string `json:"identity_issuer"`
			OperatorIssuer string `json:"operator_issuer"`
			TenantID       string `json:"tenant_id"`
		}{
			Version:        "baseharbor.management-installation/v1",
			InstallationID: id,
			IdentityIssuer: issuer,
			TenantID:       tenant.TenantID,
		})
	})
	revoker, ok := machineExecutor.(workstationRevocationReader)
	if !ok {
		return errors.New("Core device-only API requires live OpenBao revocation status")
	}
	device, err := workstationDeviceMiddleware(deps.pool, cfg.TLSClientCAFile, revoker, deviceMux)
	if err != nil {
		return err
	}
	mux.Handle("/api/", device)
	return nil
}
