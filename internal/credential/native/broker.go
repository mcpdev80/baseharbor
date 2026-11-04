package native

import (
	"context"
	"errors"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/credential"
)

// Broker is the credential.Broker implementation that reuses existing
// host-native authentication sources instead of prompting. It never returns
// secret material to the caller: Resolve reports whether a native source
// resolved a credential, never its value.
type Broker struct {
	// Git resolves an https://host entry through the configured git credential
	// helper. When nil the git check is skipped.
	Git GitResoler
	// SSH reports whether an SSH agent can serve key-based auth for host. When
	// nil the SSH check is skipped.
	SSH SSHKeyChecker
}

// GitResoler resolves a git credential for a host. Kept narrow so the broker
// depends only on the part of GitCredentialSource it calls.
type GitResoler interface {
	Resolve(ctx context.Context, host string) (resolved bool, err error)
}

// SSHKeyChecker reports whether an SSH agent has a key usable for host. Kept
// narrow so the broker depends only on the part of SSHAgentSource it calls.
type SSHKeyChecker interface {
	HasKey(ctx context.Context, host string) (bool, error)
}

// ErrNoNativeCredential documents the documented-absent case: no native source
// resolved a credential for the requested host. Never prompts; never leaks
// material.
var ErrNoNativeCredential = errors.New("native: no native credential resolved for host")

// Compile-time proof the native broker satisfies the accepted credential.Broker
// consumption path.
var _ credential.Broker = Broker{}

// Resolve implements credential.Broker. It reports whether a native source
// (git helper or SSH agent) resolved a credential for host; the resolved value
// is never returned. A missing source fails closed with ErrNoNativeCredential,
// never prompting and never leaking material.
func (b Broker) Resolve(ctx context.Context, ref string, host string) (credential.Data, error) {
	ref = trim(ref)
	host = trim(host)
	if ref == "" {
		return credential.Data{}, credential.ErrEmptyRef
	}
	if host == "" {
		return credential.Data{}, ErrNoNativeCredential
	}

	if b.Git != nil {
		if resolved, err := b.Git.Resolve(ctx, host); err == nil && resolved {
			return credential.Data{}, nil
		}
	}
	if b.SSH != nil {
		if has, err := b.SSH.HasKey(ctx, host); err == nil && has {
			return credential.Data{}, nil
		}
	}

	return credential.Data{}, ErrNoNativeCredential
}

// trim returns the trimmed value or the empty string.
func trim(s string) string {
	return strings.TrimSpace(s)
}
