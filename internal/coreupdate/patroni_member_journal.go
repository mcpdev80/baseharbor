package coreupdate

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// PatroniMemberJournal stores each member's progress in the existing,
// fsync-backed and release-bound Core journal. A new desired image pin
// generates distinct keys, preventing stale verified steps from being reused.
type PatroniMemberJournal struct {
	Path         string
	Release      string
	Installation string
	Scope        string
	Desired      Desired
}

func (j PatroniMemberJournal) delta(name string) (Delta, error) {
	if j.Path == "" || j.Release == "" || j.Installation == "" || j.Scope == "" || !strings.HasPrefix(name, "postgres-member-") ||
		!validDigest(j.Desired.Digest) || j.Desired.Kind != SQL || j.Desired.Image == "" || j.Desired.Version == "" {
		return Delta{}, errors.New("incomplete or unsafe Patroni member journal identity")
	}
	switch name {
	case "postgres-member-1", "postgres-member-2", "postgres-member-3":
	default:
		return Delta{}, fmt.Errorf("foreign Patroni member %q", name)
	}
	return Delta{Installed: Realization{Kind: SQL, Installation: j.Installation, Scope: j.Scope, Instance: name, Owner: "baseharbor"}, Desired: j.Desired}, nil
}
func (j PatroniMemberJournal) StepState(ctx context.Context, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	delta, err := j.delta(name)
	if err != nil {
		return "", err
	}
	stored, err := LoadJournal(j.Path, j.Release)
	if err != nil {
		return "", err
	}
	return stored.Steps[JournalKey(delta)], nil
}
func (j PatroniMemberJournal) Record(ctx context.Context, name, state string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delta, err := j.delta(name)
	if err != nil {
		return err
	}
	stored, err := LoadJournal(j.Path, j.Release)
	if err != nil {
		return err
	}
	return stored.Record(j.Path, delta, state)
}
