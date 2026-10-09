package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// patroniCoreRollingOps is the runtime-side adapter for the DCS-gated Core
// state machine. Only owned Core member services may be force-recreated.
// This adapter is activated only by rollOwnedCoreHAPostgres after native
// physical backup and an authenticated, isolated-restore-verified DCS proof.
type patroniCoreRollingOps struct {
	runtime bhruntime.RuntimeProvider
	files   bhruntime.Files
	journal coreupdate.PatroniMemberJournal
	backup  coreupdate.StreamRecoveryPoint
}

func (o *patroniCoreRollingOps) VerifyRecovery(context.Context) error {
	return coreupdate.VerifyPostgresBasebackup(o.backup, "18")
}
func (o *patroniCoreRollingOps) Inspect(ctx context.Context) ([]coreupdate.PatroniMemberState, error) {
	return inspectPatroniMembers(ctx, o.runtime, o.files)
}
func (o *patroniCoreRollingOps) verifyOwned(name string) error {
	for _, svc := range o.files.PostgresMembers() {
		if svc == name {
			return nil
		}
	}
	return fmt.Errorf("foreign Patroni member %q", name)
}
func (o *patroniCoreRollingOps) Recreate(ctx context.Context, name string) error {
	if err := o.verifyOwned(name); err != nil {
		return err
	}
	env, err := bhruntime.RuntimeEnvironment(o.files)
	if err != nil {
		return err
	}
	return o.runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, o.files.Project, filepath.Dir(o.files.Compose), env, []string{name}, o.files.Compose)
}
func (o *patroniCoreRollingOps) VerifyMemberImage(ctx context.Context, name string) error {
	if err := o.verifyOwned(name); err != nil {
		return err
	}
	identity, err := o.runtime.ProjectServiceImageIdentity(ctx, o.files.Project, name)
	if err != nil {
		return err
	}
	digest := identity.Digest
	if i := strings.Index(digest, "@sha256:"); i >= 0 {
		digest = digest[i+1:]
	}
	if digest != o.journal.Desired.Digest || strings.TrimSpace(identity.Reference) != o.journal.Desired.Image {
		return fmt.Errorf("Patroni member %s not running the desired immutable image and reference", name)
	}
	return nil
}
func (o *patroniCoreRollingOps) Record(ctx context.Context, name, state string) error {
	return o.journal.Record(ctx, name, state)
}
func (o *patroniCoreRollingOps) StepState(ctx context.Context, name string) (string, error) {
	return o.journal.StepState(ctx, name)
}
func (o *patroniCoreRollingOps) RecoverInterrupted(ctx context.Context, name, state string) error {
	if err := o.verifyOwned(name); err != nil {
		return err
	}
	if state != "applying" && state != "apply_failed" && state != "verify_failed" {
		return errors.New("unrecognized Patroni recovery state")
	}
	// Recreate is intentionally not replayed after an ambiguous restart. Verify
	// the observed image and all member roles, otherwise stop for operator
	// recovery with the still-preserved physical/DCS snapshots.
	if err := o.VerifyMemberImage(ctx, name); err != nil {
		return fmt.Errorf("UNSUPPORTED: interrupted Patroni member requires operator reconciliation: %w", err)
	}
	members, err := o.Inspect(ctx)
	if err != nil {
		return err
	}
	_, _, err = coreupdate.VerifyPatroniQuorum(ctx, members, 0)
	return err
}
func (o *patroniCoreRollingOps) Switchover(ctx context.Context, old, candidate string) error {
	if old == candidate {
		return errors.New("Patroni switchover requires distinct members")
	}
	if err := o.verifyOwned(old); err != nil {
		return err
	}
	if err := o.verifyOwned(candidate); err != nil {
		return err
	}
	// A targeted Patroni switchover request is distinct from a forced kill or
	// a Compose project-wide stop. POST arguments carry only service names.
	const script = "import json,sys,urllib.request\nbody=json.dumps({'leader':sys.argv[1],'candidate':sys.argv[2]}).encode('utf-8')\nreq=urllib.request.Request('http://127.0.0.1:8008/switchover',data=body,headers={'Content-Type':'application/json'},method='POST')\nwith urllib.request.urlopen(req,timeout=20) as response:\n assert response.status in (200,202)"
	_, err := o.runtime.ExecProject(ctx, o.files.Project, o.files.Compose, o.files.Env, old, "python3", "-c", script, old, candidate)
	if err != nil {
		return fmt.Errorf("Patroni native switchover rejected: %w", err)
	}
	return nil
}
