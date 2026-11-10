package workstationenrollment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// MaxCertificateTTL is the longest workstation certificate lifetime supported
// by the enrollment contract. Five years avoids routine developer re-login.
const MaxCertificateTTL = serviceaccess.MaxWorkstationCertificateTTL

// ParseCertificateTTL accepts Go duration units plus day and year suffixes.
func ParseCertificateTTL(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	unit := time.Duration(0)
	for suffix, size := range map[string]time.Duration{"y": 365 * 24 * time.Hour, "d": 24 * time.Hour} {
		if strings.HasSuffix(value, suffix) {
			unit = size
			value = strings.TrimSuffix(value, suffix)
			break
		}
	}
	var ttl time.Duration
	var err error
	if unit != 0 {
		var count int64
		count, err = strconv.ParseInt(value, 10, 64)
		if err == nil && (count <= 0 || count > int64(MaxCertificateTTL/unit)) {
			err = errors.New("certificate lifetime is outside the supported range")
		}
		if err == nil {
			ttl = time.Duration(count) * unit
		}
	} else {
		ttl, err = time.ParseDuration(value)
	}
	if err != nil || ttl <= 0 || ttl > MaxCertificateTTL || ttl%time.Second != 0 {
		return 0, errors.New("certificate lifetime must be whole seconds and no more than five years")
	}
	return ttl, nil
}

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

// DigestForConsume exposes the shared credential digest to the durable
// persistence layer (internal/database). The one-use store keeps only digests;
// the HTTP adapter and the production store must hash the same bearer the same
// way so a persisted grant matches the presented credential.
func DigestForConsume(value string) string { return digest(value) }

// grant is the persisted authorization state (digests only, never the bearer
// credential or nonce).
type grant struct {
	scope          Scope
	tokenDigest    string
	nonceDigest    string
	expiresAt      time.Time
	certificateTTL time.Duration
}
