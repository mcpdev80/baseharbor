package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// classifyOwnedHAPostgresRollingInventory admits a mixed Spilo image set only
// when an original release-pinned image and the desired immutable image are
// the ONLY two identities observed, and a durable member journal proves that
// a previously admitted roll is being resumed.
func classifyOwnedHAPostgresRollingInventory(ctx context.Context, state coreinstallation.State, release string, images []bhruntime.ImageIdentity, pin coreupdate.BackingPin) (coreupdate.Delta, error) {
	if !state.Spec.HA || len(images) != 3 {
		return coreupdate.Delta{}, errors.New("HA PostgreSQL requires three inventoried members")
	}
	allSame := true
	for _, image := range images[1:] {
		if image.Reference != images[0].Reference || image.Digest != images[0].Digest {
			allSame = false
		}
	}
	if allSame {
		id := images[0]
		digest := strings.TrimSpace(id.Digest)
		if at := strings.Index(digest, "@sha256:"); at >= 0 {
			digest = digest[at+1:]
		}
		return coreupdate.ClassifyHAPostgresPin(coreupdate.Realization{
			Kind: coreupdate.SQL, Installation: state.ID, Scope: "shared", Instance: "postgres-member-1",
			Owner: "baseharbor", Image: strings.TrimSpace(id.Reference), Digest: digest,
		}, pin), nil
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return coreupdate.Delta{}, err
	}
	if target.Name != state.Spec.Target || target.RuntimeProvider != state.Spec.Runtime {
		return coreupdate.Delta{}, errors.New("HA PostgreSQL inventory does not belong to selected target")
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return coreupdate.Delta{}, err
	}
	journalPath := filepath.Join(root, "core-updates", safeVersionPathPart(release), "patroni-members.json")
	if _, err := os.Lstat(journalPath); err != nil {
		return coreupdate.Delta{}, fmt.Errorf("mixed Patroni versions without durable member journal: %w", err)
	}
	desired := coreupdate.Desired{Kind: coreupdate.SQL, Image: pin.Image, Digest: pin.Digest, Version: pin.Version}
	journal := coreupdate.PatroniMemberJournal{
		Path: journalPath, Release: release, Installation: state.ID, Scope: "shared", Desired: desired,
	}
	var previous *coreupdate.Realization
	upgraded := 0
	for i, id := range images {
		name := fmt.Sprintf("postgres-member-%d", i+1)
		digest := strings.TrimSpace(id.Digest)
		if at := strings.Index(digest, "@sha256:"); at >= 0 {
			digest = digest[at+1:]
		}
		ref := strings.TrimSpace(id.Reference)
		step, err := journal.StepState(ctx, name)
		if err != nil {
			return coreupdate.Delta{}, err
		}
		if ref == pin.Image && digest == pin.Digest {
			if step != "verified" && step != "applying" && step != "verify_failed" {
				return coreupdate.Delta{}, fmt.Errorf("unattested upgraded Patroni member %s", name)
			}
			upgraded++
			continue
		}
		if step == "verified" {
			return coreupdate.Delta{}, fmt.Errorf("verified Patroni member %s does not match desired image", name)
		}
		if step != "" && step != "applying" && step != "apply_failed" && step != "verify_failed" {
			return coreupdate.Delta{}, fmt.Errorf("unknown interrupted Patroni member %s state", name)
		}
		if previous == nil {
			previous = &coreupdate.Realization{
				Kind: coreupdate.SQL, Installation: state.ID, Scope: "shared", Instance: "postgres-member-1",
				Owner: "baseharbor", Image: ref, Digest: digest,
			}
		} else if previous.Image != ref || previous.Digest != digest {
			return coreupdate.Delta{}, errors.New("more than two foreign/old Spilo image identities in HA inventory")
		}
	}
	if previous == nil || upgraded == 0 {
		return coreupdate.Delta{}, errors.New("mixed Patroni inventory has no owned previous and desired images")
	}
	delta := coreupdate.ClassifyHAPostgresPin(*previous, pin)
	if delta.Classification != coreupdate.BackupRequired {
		return coreupdate.Delta{}, errors.New("mixed Patroni members do not represent an approved Spilo patch migration")
	}
	return delta, nil
}
