package workstationenrollment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"time"
)

var ErrDenied = errors.New("workstation enrollment is invalid, expired, or consumed")

// tenantID matches a UUID v4 tenant (installation) identifier.
var tenantID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// segment matches a single DNS-label-style identifier.
var segment = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Scope is the workstation registration context selected and authorized by the
// installation before a one-use bootstrap is created. It is distinct from the
// Core connector scope used by internal/targetenrollment: it is bound to the
// local installation and a workstation label, never to a connector tenant,
// Target, or node.
type Scope struct {
	TenantID string // installation UUID
	Station  string // workstation label (single DNS label)
}

// Identity is the SPIFFE identity the enrolled workstation receives. It is
// installation-scoped, not connector-scoped, so a workstation leaf can never
// be read as a connector/node identity.
func (s Scope) Identity() string {
	return "spiffe://baseharbor/workstations/" + s.TenantID + "/" + s.Station
}

// Validate rejects a scope whose fields are malformed or whose station label is
// empty. A foreign or unparseable scope fails closed.
func (s Scope) Validate() error {
	if !tenantID.MatchString(s.TenantID) || !segment.MatchString(s.Station) || s.Station == "" {
		return ErrDenied
	}
	return nil
}

// digest is the SHA-256 hex of a credential. Defined once here and shared by
// the authority and the one-use store.
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// grant is the persisted authorization state (digests only, never the bearer
// credential or nonce).
type grant struct {
	scope          Scope
	tokenDigest    string
	nonceDigest    string
	expiresAt      time.Time
	certificateTTL time.Duration
}
