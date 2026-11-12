package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/health"
)

func inspectTUIControlPlane(ctx context.Context) (controlPlaneReport, error) {
	if target, err := effectiveTarget(ctx); err == nil && remoteCoreTarget(target) {
		return remoteControlPlaneStatus(ctx, target)
	}
	return inspectControlPlane(ctx)
}

// Remote dashboards use the owning Core's read-only operations. Workstation
// deployment caches and Connector sessions are not remote runtime authority.
func (m tuiModel) loadRemoteStatus(target deployment.ResolvedTarget) tuiStatusMsg {
	if m.coreMode {
		return m.loadRemoteCoreStatus(target)
	}
	name, environment, ok := currentRepositoryApplicationSelector()
	if !ok {
		return tuiStatusMsg{err: errors.New("remote TUI needs a valid repository baseharbor.yaml")}
	}
	if environment == "" {
		environment = "dev"
	}
	raw, err := remoteMachineRead(m.ctx, target, "status", name, environment, nil)
	if err != nil {
		return tuiStatusMsg{err: err}
	}
	var status tuiApplicationStatusResult
	if json.Unmarshal(raw, &status) != nil || status.ContractVersion != "v1" || status.Target != target.Name || status.Application != name || status.Environment != environment {
		return tuiStatusMsg{err: errors.New("remote TUI status does not match the requested application authority")}
	}
	msg := tuiStatusMsg{result: status.StatusResult, tls: status.TLS}
	raw, err = remoteMachineRead(m.ctx, target, "doctor", name, environment, nil)
	if err != nil {
		msg.err = err
		return msg
	}
	if json.Unmarshal(raw, &msg.doctor) != nil || msg.doctor.ContractVersion != "v1" || msg.doctor.Target != target.Name || msg.doctor.Application != name || msg.doctor.Environment != environment {
		msg.err = errors.New("remote TUI doctor does not match the requested application authority")
	}
	return msg
}

func (m tuiModel) loadRemoteCoreStatus(target deployment.ResolvedTarget) tuiStatusMsg {
	status, err := remoteControlPlaneStatus(m.ctx, target)
	if err != nil {
		return tuiStatusMsg{err: err}
	}
	msg := tuiStatusMsg{coreStatus: renderCoreTUIStatus(target.Name, target.RuntimeProvider, target.AccessProvider, targetSelectionOrigin(m.ctx), status)}
	var overview bytes.Buffer
	// Keep degraded status visible; the human renderer also reports readiness as
	// an error after rendering its checks.
	renderErr := renderRemoteControlPlaneStatus(m.ctx, target, status, &overview)
	if renderErr != nil && status.Ready {
		return tuiStatusMsg{err: renderErr}
	}
	msg.coreView = overview.String()
	doctor, err := remoteControlPlaneDoctor(m.ctx, target)
	if err != nil {
		msg.err = err
		return msg
	}
	checks := make([]health.Check, 0, len(doctor.Checks))
	for _, check := range doctor.Checks {
		checks = append(checks, health.Check{Name: check.Name, OK: check.Ready, Message: check.Message})
	}
	if !doctor.Ready {
		checks = append(checks, health.Check{Name: "Remote Core readiness", OK: false, Message: "Core reports not ready; run baha doctor --verbose"})
	}
	msg.coreDoctor = renderCoreTUIDoctor(checks)
	return msg
}
