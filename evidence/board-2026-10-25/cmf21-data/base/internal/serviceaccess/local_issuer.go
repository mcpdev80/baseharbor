package serviceaccess

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// LocalIssuer is the software realization of the existing service-access
// issuer contract. It needs no runtime service. Its private authority remains
// in protected Target state and is never projected into application bindings.
// Construction is read-only; only EnsureLocalIssuer initializes authority.
type LocalIssuer struct{ Dir string }

// CheckLocalIssuer performs a read-only preflight. Missing authority is a new
// realization; partial or unprotected authority must be recovered, not replaced.
func CheckLocalIssuer(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("local issuer requires canonical private state")
	}
	if _, err := os.Lstat(filepath.Join(dir, "issuer.pem")); err == nil {
		_, _, _, err = (&LocalIssuer{Dir: dir}).load()
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != ".installation.lock" {
			return errors.New("partial local issuer state requires recovery")
		}
	}
	return nil
}

func EnsureLocalIssuer(dir string) (*LocalIssuer, error) {
	i := &LocalIssuer{Dir: dir}
	if err := CheckLocalIssuer(dir); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("local issuer requires canonical private state")
	}
	unlock, err := coreinstallation.AcquireLifecycleLock(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	path := filepath.Join(dir, "issuer.pem")
	if _, err := os.Lstat(path); err == nil {
		_, _, _, err = i.load()
		return i, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name() != ".installation.lock" {
			return nil, errors.New("partial local issuer state requires recovery")
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := localIssuerSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	ca := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "BaseHarbor Target Service Authority"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(5 * 365 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	data = append(data, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})...)
	if err := writeLocalIssuerAtomic(path, data); err != nil {
		return nil, err
	}
	return i, nil
}

func (i *LocalIssuer) load() (*x509.Certificate, *ecdsa.PrivateKey, string, error) {
	info, err := os.Lstat(i.Dir)
	if err != nil {
		return nil, nil, "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, nil, "", errors.New("local issuer requires canonical private state")
	}
	path := filepath.Join(i.Dir, "issuer.pem")
	info, err = os.Lstat(path)
	if err != nil {
		return nil, nil, "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return nil, nil, "", errors.New("local issuer authority is unprotected or invalid")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", err
	}
	certBlock, rest := pem.Decode(data)
	keyBlock, trailing := pem.Decode(rest)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" || keyBlock == nil || keyBlock.Type != "PRIVATE KEY" || len(trailing) != 0 {
		return nil, nil, "", errors.New("local issuer authority is incomplete")
	}
	ca, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, "", err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, "", err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || !ca.IsCA || !ca.BasicConstraintsValid || ca.KeyUsage&x509.KeyUsageCertSign == 0 || ca.CheckSignatureFrom(ca) != nil || !time.Now().Before(ca.NotAfter.Add(-90*24*time.Hour)) || time.Now().Before(ca.NotBefore) {
		return nil, nil, "", errors.New("local issuer authority requires recovery or explicit trust rotation")
	}
	public, ok := ca.PublicKey.(*ecdsa.PublicKey)
	if !ok || !public.Equal(&key.PublicKey) {
		return nil, nil, "", errors.New("local issuer certificate/key mismatch")
	}
	fingerprint := sha256.Sum256(ca.Raw)
	return ca, key, fmt.Sprintf("local-pki:%x", fingerprint), nil
}

func (i *LocalIssuer) TrustBundle(ctx context.Context) (TrustBundle, error) {
	if err := ctx.Err(); err != nil {
		return TrustBundle{}, err
	}
	ca, _, ref, err := i.load()
	if err != nil {
		return TrustBundle{}, err
	}
	return TrustBundle{IssuerReference: ref, PEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})}, nil
}

func (i *LocalIssuer) Status(ctx context.Context) (IssuerStatus, error) {
	trust, err := i.TrustBundle(ctx)
	return IssuerStatus{IssuerReference: trust.IssuerReference, Ready: err == nil}, err
}

func (i *LocalIssuer) Issue(ctx context.Context, request CertificateRequest) (IssuedCertificate, error) {
	if err := ctx.Err(); err != nil {
		return IssuedCertificate{}, err
	}
	ca, signingKey, ref, err := i.load()
	if err != nil {
		return IssuedCertificate{}, err
	}
	if request.CommonName == "" || request.TTL <= 0 || request.TTL > 90*24*time.Hour {
		return IssuedCertificate{}, errors.New("local service certificate requires identity and TTL of at most 90 days")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return IssuedCertificate{}, err
	}
	serial, err := localIssuerSerial()
	if err != nil {
		return IssuedCertificate{}, err
	}
	now := time.Now().UTC()
	leaf := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: request.CommonName}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(request.TTL), KeyUsage: x509.KeyUsageDigitalSignature, DNSNames: request.DNSNames, IPAddresses: request.IPAddresses, URIs: request.URIs, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if len(request.DNSNames) > 0 || len(request.IPAddresses) > 0 {
		leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, signingKey)
	if err != nil {
		return IssuedCertificate{}, err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return IssuedCertificate{}, err
	}
	return IssuedCertificate{IssuerReference: ref, Certificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), PrivateKey: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), IssuingCA: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), Serial: serial.Text(16), ExpiresAt: leaf.NotAfter}, nil
}

func (i *LocalIssuer) Renew(ctx context.Context, current IssuedCertificate, request CertificateRequest) (IssuedCertificate, error) {
	ca, _, ref, err := i.load()
	if err != nil {
		return IssuedCertificate{}, err
	}
	block, _ := pem.Decode(current.Certificate)
	if block == nil || current.IssuerReference != ref {
		return IssuedCertificate{}, errors.New("local issuer refuses foreign renewal")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil || leaf.CheckSignatureFrom(ca) != nil {
		return IssuedCertificate{}, errors.New("local issuer refuses unverifiable renewal")
	}
	list, err := i.revocations(ca)
	if err != nil {
		return IssuedCertificate{}, err
	}
	if list != nil {
		for _, entry := range list.RevokedCertificateEntries {
			if entry.SerialNumber.Cmp(leaf.SerialNumber) == 0 {
				return IssuedCertificate{}, errors.New("local issuer refuses revoked renewal")
			}
		}
	}
	return i.Issue(ctx, request)
}

func (i *LocalIssuer) Revoke(ctx context.Context, serial string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := coreinstallation.AcquireLifecycleLock(i.Dir)
	if err != nil {
		return err
	}
	defer unlock()
	ca, key, _, err := i.load()
	if err != nil {
		return err
	}
	number, ok := new(big.Int).SetString(serial, 16)
	if !ok || number.Sign() <= 0 || number.BitLen() > 159 {
		return errors.New("invalid local certificate serial")
	}
	path := filepath.Join(i.Dir, "revocations.pem")
	var entries []x509.RevocationListEntry
	sequence := big.NewInt(1)
	list, err := i.revocations(ca)
	if err != nil {
		return err
	}
	if list != nil {
		entries = list.RevokedCertificateEntries
		sequence.Add(list.Number, big.NewInt(1))
		for _, entry := range entries {
			if entry.SerialNumber.Cmp(number) == 0 {
				return nil
			}
		}
	}
	now := time.Now().UTC()
	entries = append(entries, x509.RevocationListEntry{SerialNumber: number, RevocationTime: now})
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{Number: sequence, ThisUpdate: now, NextUpdate: now.Add(24 * time.Hour), RevokedCertificateEntries: entries}, ca, key)
	if err != nil {
		return err
	}
	return writeLocalIssuerAtomic(path, pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}))
}

// revocations validates protected state before reading it, including during
// renewal. An expired publication still retains every permanent revocation.
func (i *LocalIssuer) revocations(ca *x509.Certificate) (*x509.RevocationList, error) {
	path := filepath.Join(i.Dir, "revocations.pem")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1024*1024 {
		return nil, errors.New("unprotected local revocation state")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "X509 CRL" || len(rest) != 0 {
		return nil, errors.New("invalid local revocation state")
	}
	list, err := x509.ParseRevocationList(block.Bytes)
	if err != nil || list.Number == nil || list.Number.Sign() < 0 || list.CheckSignatureFrom(ca) != nil {
		return nil, errors.New("unverifiable local revocation state")
	}
	return list, nil
}

func writeLocalIssuerAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".issuer-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func localIssuerSerial() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 159))
	if err != nil {
		return nil, err
	}
	return serial.Add(serial, big.NewInt(1)), nil
}
