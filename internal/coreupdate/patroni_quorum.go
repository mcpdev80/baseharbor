package coreupdate

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// PatroniMemberState is a verified, non-secret snapshot from Patroni's
// /primary and /replica endpoints. Unknown members must never be inferred.
type PatroniMemberState struct {
	Name      string
	Primary   bool
	Replica   bool
	ReplayLag int64
	Healthy   bool
}

// VerifyPatroniQuorum checks a complete three-member HA snapshot before a
// database upgrade may be considered. It deliberately does not authorize an
// image mutation: a durable logical/physical backup and provider-native rolling
// lifecycle are additional independent requirements.
func VerifyPatroniQuorum(ctx context.Context, members []PatroniMemberState, maxReplayLag int64) (string, []string, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if maxReplayLag < 0 || len(members) != 3 {
		return "", nil, errors.New("Patroni HA requires three verified members and a nonnegative replay-lag budget")
	}
	seen := map[string]bool{}
	primary := ""
	replicas := make([]string, 0, 2)
	for _, member := range members {
		if member.Name == "" || seen[member.Name] || !member.Healthy || member.Primary == member.Replica || member.ReplayLag < 0 {
			return "", nil, fmt.Errorf("Patroni member %q has unknown, duplicate or unsafe state", member.Name)
		}
		seen[member.Name] = true
		if member.Primary {
			if primary != "" {
				return "", nil, errors.New("Patroni split brain: multiple primaries")
			}
			primary = member.Name
		} else {
			if member.ReplayLag > maxReplayLag {
				return "", nil, fmt.Errorf("Patroni replica %s exceeds permitted replay lag", member.Name)
			}
			replicas = append(replicas, member.Name)
		}
	}
	if primary == "" || len(replicas) != 2 {
		return "", nil, errors.New("Patroni leader or two healthy replicas missing")
	}
	sort.Strings(replicas)
	return primary, replicas, nil
}
