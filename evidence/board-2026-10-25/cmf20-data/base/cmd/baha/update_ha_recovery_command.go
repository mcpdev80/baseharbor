package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

func recoverNativeCoreHA(ctx context.Context, release string, out io.Writer) error {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
	unlock, err := coreinstallation.AcquireLifecycleLock(root)
	if err != nil {
		return err
	}
	defer unlock()
	state, err := coreinstallation.Load(root)
	if err != nil {
		return err
	}
	if !state.Ready || !state.Spec.HA || state.ID == "" || state.Spec.Target != target.Name || state.Spec.Runtime != target.RuntimeProvider {
		return errors.New("HA recovery requires the matching, owned Core and selected Target")
	}
	rt, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return err
	}
	journal, err := coreUpdateJournal(root, release, false)
	if err != nil {
		return err
	}
	if err := recoverOwnedCoreHA(ctx, rt, files, journal, state.ID, target.Name, release); err != nil {
		return err
	}
	fmt.Fprintln(out, "[OK] Core HA recovery: PostgreSQL SQL readiness, restored DCS quorum and Patroni replication verified; original data volumes retained")
	return nil
}
