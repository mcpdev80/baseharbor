package targetenrollment

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"regexp"
	"strings"
	"time"
)

// NodeRegistry admits only a currently active certificate bound to the exact
// persisted tenant/Target/node/runtime. A trusted CA alone is not admission.
type NodeRegistry interface {
	AdmitCertificate(context.Context, Scope, string, time.Time) error
}

var certificateSerial = regexp.MustCompile("^[0-9a-f]{1,128}$")

func NormalizeCertificateSerial(serial string) (string, error) {
	serial = strings.TrimLeft(strings.ReplaceAll(strings.ToLower(serial), ":", ""), "0")
	if !certificateSerial.MatchString(serial) {
		return "", ErrDenied
	}
	return serial, nil
}

// AdmitTLSNode must be called after a server-side RequireAndVerifyClientCert
// TLS 1.3 handshake and again when binding Hello/capabilities to the Core scope.
// It does not advertise a capability or execute a runtime request.
func AdmitTLSNode(ctx context.Context, state tls.ConnectionState, scope Scope, registry NodeRegistry) error {
	if ctx.Err() != nil || registry == nil || scope.Validate() != nil ||
		state.Version < tls.VersionTLS13 || !state.HandshakeComplete || len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return ErrDenied
	}
	leaf := state.PeerCertificates[0]
	if leaf == nil {
		return ErrDenied
	}
	verified := false
	for _, chain := range state.VerifiedChains {
		if len(chain) != 0 && chain[0] != nil && bytes.Equal(chain[0].Raw, leaf.Raw) {
			verified = true
		}
	}
	now := time.Now()
	if !verified || leaf.IsCA || leaf.Subject.CommonName != scope.Identity() ||
		len(leaf.URIs) != 1 || leaf.URIs[0].String() != scope.Identity() ||
		len(leaf.DNSNames) != 0 || len(leaf.IPAddresses) != 0 || len(leaf.EmailAddresses) != 0 ||
		len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth ||
		len(leaf.UnknownExtKeyUsage) != 0 || leaf.KeyUsage != x509.KeyUsageDigitalSignature ||
		leaf.NotBefore.After(now) || !leaf.NotAfter.After(now) || leaf.SerialNumber == nil || leaf.SerialNumber.Sign() <= 0 {
		return ErrDenied
	}
	if verifyCurrentNodeTrust(ctx, registry, state.PeerCertificates, now) != nil {
		return ErrDenied
	}
	serial, err := NormalizeCertificateSerial(leaf.SerialNumber.Text(16))
	if err != nil || registry.AdmitCertificate(ctx, scope, serial, leaf.NotAfter) != nil || ctx.Err() != nil {
		return ErrDenied
	}
	return nil
}
