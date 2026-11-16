// Package onboardingbundle defines a strict portable one-use workstation invitation.
package onboardingbundle

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

const Contract = "baseharbor.workstation-onboard/v1"

var coreInstallationUUID = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")
var coreTargetName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

const MaxBytes int64 = 64 << 10

var ErrInvalid = errors.New("invalid or expired workstation onboarding bundle")

// Bundle is a *transport envelope*, not permission to bypass Core operator
// authentication. The Core must mint Token and Nonce for exactly this Scope,
// persist their digests durably and consume the grant exactly once on enroll.
// The Workstation produces its own CSR and private key.
type Bundle struct {
	ContractVersion    string    `json:"contract_version"`
	CoreURL            string    `json:"core_url"`
	CAPEM              string    `json:"ca_pem"`
	InstallationID     string    `json:"installation_id"`
	CoreInstallationID string    `json:"core_installation_id"`
	CoreTarget         string    `json:"core_target"`
	RuntimeProvider    string    `json:"runtime_provider"`
	Station            string    `json:"station"`
	Token              string    `json:"token"`
	Nonce              string    `json:"nonce"`
	ExpiresAt          time.Time `json:"expires_at"`
}

func (b Bundle) Validate(now time.Time) error {
	if b.ContractVersion != Contract {
		return fmt.Errorf("%w: unsupported contract version", ErrInvalid)
	}
	if err := (ws.Scope{TenantID: b.InstallationID, Station: b.Station}).Validate(); err != nil {
		return fmt.Errorf("%w: invalid installation or workstation", ErrInvalid)
	}
	if !coreInstallationUUID.MatchString(b.CoreInstallationID) || b.CoreInstallationID == b.InstallationID ||
		!coreTargetName.MatchString(b.CoreTarget) ||
		(b.RuntimeProvider != "docker" && b.RuntimeProvider != "podman") {
		return fmt.Errorf("%w: missing distinct Core installation identity or invalid runtime", ErrInvalid)
	}
	u, err := url.Parse(b.CoreURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Host == "" {
		return fmt.Errorf("%w: Core address must be an HTTPS origin", ErrInvalid)
	}
	if strings.Contains(u.Host, "@") || strings.Contains(u.Host, "%") {
		return fmt.Errorf("%w: invalid Core authority", ErrInvalid)
	}
	host := u.Hostname()
	if host == "" || strings.ContainsAny(host, " /\\") || net.ParseIP(host) == nil && strings.Contains(host, ":") {
		return fmt.Errorf("%w: invalid Core host", ErrInvalid)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(b.CAPEM)) {
		return fmt.Errorf("%w: missing valid public Core CA", ErrInvalid)
	}
	block, _ := pem.Decode([]byte(b.CAPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return fmt.Errorf("%w: CA material is not a public certificate", ErrInvalid)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !cert.IsCA || cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return fmt.Errorf("%w: trust anchor is not a signing CA", ErrInvalid)
	}
	for _, credential := range []string{b.Token, b.Nonce} {
		if len(credential) < 32 || len(credential) > 512 || strings.TrimSpace(credential) != credential || strings.ContainsAny(credential, "\r\n") {
			return fmt.Errorf("%w: missing one-use enrollment grant", ErrInvalid)
		}
	}
	if b.ExpiresAt.IsZero() || !b.ExpiresAt.After(now) || b.ExpiresAt.After(now.Add(15*time.Minute)) {
		return fmt.Errorf("%w: enrollment grant expired or exceeds 15-minute lifetime", ErrInvalid)
	}
	if strings.Contains(b.CAPEM, "PRIVATE KEY") {
		return fmt.Errorf("%w: private key is forbidden", ErrInvalid)
	}
	return nil
}

// Write securely creates an invitation only once. Refusing overwrite prevents
// accidental replacement of an existing grant or following a symlink.
func Write(path string, b Bundle, now time.Time) (err error) {
	if err := b.Validate(now); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if len(payload) > int(MaxBytes) {
		return fmt.Errorf("%w: bundle exceeds maximum size", ErrInvalid)
	}
	// The destination directory must already be private to its owner.
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !parent.IsDir() || parent.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: output directory must be private", ErrInvalid)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	if _, err = io.Copy(file, bytes.NewReader(payload)); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// Read refuses symlinks, world-readable bundles, trailing data, and files over
// the bounded limit before their bearer grants are ever sent over the network.
func Read(path string, now time.Time) (Bundle, error) {
	var empty Bundle
	fi, err := os.Lstat(path)
	if err != nil {
		return empty, err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0o077 != 0 || fi.Size() > MaxBytes {
		return empty, fmt.Errorf("%w: bundle must be a private regular file (0600, <=64 KiB)", ErrInvalid)
	}
	f, err := os.Open(path)
	if err != nil {
		return empty, err
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, MaxBytes+1))
	dec.DisallowUnknownFields()
	var b Bundle
	if err := dec.Decode(&b); err != nil {
		return empty, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return empty, fmt.Errorf("%w: unexpected trailing content", ErrInvalid)
	}
	if err := b.Validate(now); err != nil {
		return empty, err
	}
	return b, nil
}
