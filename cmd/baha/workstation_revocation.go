package main

import (
	"context"
	"errors"
	"time"

	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
)

// WorkstationCertificateRevoked is read-only: the Core's existing OpenBao
// manager CRL, not a workstation assertion, is the final revocation owner.
// Failure to read the issuer denies device authentication.
func (e *bahaMachineExecutor) WorkstationCertificateRevoked(ctx context.Context, serial string) (bool, error) {
	if e == nil || e.coreAuthority == nil || serial == "" ||
		(e.coreAuthority.AccessProvider != "" && e.coreAuthority.AccessProvider != "local") {
		return false, errors.New("pinned local Core workstation revocation authority missing")
	}
	ctx = withTargetOverride(ctx, e.coreAuthority.Name)
	check, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider, files, err := openBaoRuntime(check)
	if err != nil {
		return false, errors.New("managed Core OpenBao revocation service unavailable")
	}
	issuer := platformopenbao.NewServiceIssuer(provider, files)
	return issuer.RevokedStatus(check, serial)
}
