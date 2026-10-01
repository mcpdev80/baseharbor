package externalprovider

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CertificatePair struct {
	Certificate string   `json:"certificate"`
	PrivateKey  string   `json:"private_key"`
	DNSNames    []string `json:"dns_names,omitempty"`
	ClientAuth  bool     `json:"client_auth,omitempty"`
	ServerAuth  bool     `json:"server_auth,omitempty"`
	NotBefore   string   `json:"not_before,omitempty"`
	NotAfter    string   `json:"not_after,omitempty"`
}

type CertificateDiscovery struct {
	Directory       string            `json:"directory"`
	CAReferences    []string          `json:"ca_references,omitempty"`
	Pairs           []CertificatePair `json:"pairs,omitempty"`
	HostnameMatches []CertificatePair `json:"hostname_matches,omitempty"`
	ClientPairs     []CertificatePair `json:"client_pairs,omitempty"`
}

type parsedCertificate struct {
	Path string
	Cert *x509.Certificate
}

type parsedPrivateKey struct {
	Path string
	Key  crypto.PrivateKey
}

func DiscoverCertificateDirectory(dir, hostname string) (CertificateDiscovery, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return CertificateDiscovery{}, errors.New("certificate directory is required")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return CertificateDiscovery{}, fmt.Errorf("inspect certificate directory: %w", err)
	}
	if !info.IsDir() {
		return CertificateDiscovery{}, fmt.Errorf("certificate directory %q is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return CertificateDiscovery{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var certs []parsedCertificate
	var keys []parsedPrivateKey
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		fi, err := entry.Info()
		if err != nil {
			return CertificateDiscovery{}, err
		}
		if !fi.Mode().IsRegular() {
			continue
		}
		if fi.Size() > 16<<20 {
			return CertificateDiscovery{}, fmt.Errorf("certificate directory file %q is unexpectedly large", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return CertificateDiscovery{}, err
		}
		fileCerts, fileKeys := parseCertificateMaterial(path, data)
		certs = append(certs, fileCerts...)
		keys = append(keys, fileKeys...)
	}

	result := CertificateDiscovery{Directory: filepath.Clean(dir)}
	seenCA := map[string]struct{}{}
	for _, item := range certs {
		if item.Cert.IsCA {
			if _, exists := seenCA[item.Path]; !exists {
				result.CAReferences = append(result.CAReferences, item.Path)
				seenCA[item.Path] = struct{}{}
			}
		}
	}
	sort.Strings(result.CAReferences)

	now := time.Now()
	for _, item := range certs {
		if item.Cert.IsCA {
			continue
		}
		for _, key := range keys {
			match, err := certificateMatchesPrivateKey(item.Cert, key.Key)
			if err != nil {
				return CertificateDiscovery{}, err
			}
			if !match {
				continue
			}
			if now.Before(item.Cert.NotBefore) || now.After(item.Cert.NotAfter) {
				return CertificateDiscovery{}, fmt.Errorf("certificate %q is outside its validity period", item.Path)
			}
			pair := CertificatePair{
				Certificate: item.Path,
				PrivateKey:  key.Path,
				DNSNames:    append([]string(nil), item.Cert.DNSNames...),
				ClientAuth:  explicitlySupportsUsage(item.Cert, x509.ExtKeyUsageClientAuth),
				ServerAuth:  supportsUsage(item.Cert, x509.ExtKeyUsageServerAuth),
				NotBefore:   item.Cert.NotBefore.UTC().Format(time.RFC3339),
				NotAfter:    item.Cert.NotAfter.UTC().Format(time.RFC3339),
			}
			result.Pairs = append(result.Pairs, pair)
			if pair.ClientAuth {
				result.ClientPairs = append(result.ClientPairs, pair)
			}
			if strings.TrimSpace(hostname) != "" && pair.ServerAuth && item.Cert.VerifyHostname(hostname) == nil {
				result.HostnameMatches = append(result.HostnameMatches, pair)
			}
		}
	}
	sortPairs(result.Pairs)
	sortPairs(result.ClientPairs)
	sortPairs(result.HostnameMatches)

	if len(result.HostnameMatches) > 1 {
		return CertificateDiscovery{}, fmt.Errorf("certificate directory is ambiguous: %d certificate/key pairs match hostname %q", len(result.HostnameMatches), hostname)
	}
	if len(result.ClientPairs) > 1 {
		return CertificateDiscovery{}, fmt.Errorf("certificate directory is ambiguous: %d client certificate/key pairs were found", len(result.ClientPairs))
	}
	if len(certs) == 0 && len(keys) == 0 {
		return CertificateDiscovery{}, fmt.Errorf("certificate directory %q contains no supported certificate or private-key material", dir)
	}
	return result, nil
}

func parseCertificateMaterial(path string, data []byte) ([]parsedCertificate, []parsedPrivateKey) {
	var certs []parsedCertificate
	var keys []parsedPrivateKey
	rest := data
	decoded := false
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		decoded = true
		rest = next
		switch block.Type {
		case "CERTIFICATE":
			if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
				certs = append(certs, parsedCertificate{Path: path, Cert: cert})
			}
		case "PRIVATE KEY", "RSA PRIVATE KEY", "EC PRIVATE KEY":
			if key, err := parsePrivateKey(block.Bytes); err == nil {
				keys = append(keys, parsedPrivateKey{Path: path, Key: key})
			}
		}
	}
	if decoded {
		return certs, keys
	}
	if cert, err := x509.ParseCertificate(data); err == nil {
		certs = append(certs, parsedCertificate{Path: path, Cert: cert})
	}
	if key, err := parsePrivateKey(data); err == nil {
		keys = append(keys, parsedPrivateKey{Path: path, Key: key})
	}
	return certs, keys
}

func parsePrivateKey(data []byte) (crypto.PrivateKey, error) {
	if key, err := x509.ParsePKCS8PrivateKey(data); err == nil {
		switch key.(type) {
		case *rsa.PrivateKey, *ecdsa.PrivateKey, ed25519.PrivateKey:
			return key, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(data); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(data); err == nil {
		return key, nil
	}
	return nil, errors.New("unsupported private key")
}

func certificateMatchesPrivateKey(cert *x509.Certificate, key crypto.PrivateKey) (bool, error) {
	var public any
	switch k := key.(type) {
	case *rsa.PrivateKey:
		public = &k.PublicKey
	case *ecdsa.PrivateKey:
		public = &k.PublicKey
	case ed25519.PrivateKey:
		public = k.Public()
	default:
		return false, fmt.Errorf("unsupported private key type %T", key)
	}
	certDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return false, err
	}
	keyDER, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return false, err
	}
	return string(certDER) == string(keyDER), nil
}

func explicitlySupportsUsage(cert *x509.Certificate, usage x509.ExtKeyUsage) bool {
	for _, candidate := range cert.ExtKeyUsage {
		if candidate == usage {
			return true
		}
	}
	return false
}

func supportsUsage(cert *x509.Certificate, usage x509.ExtKeyUsage) bool {
	if len(cert.ExtKeyUsage) == 0 {
		return true
	}
	for _, candidate := range cert.ExtKeyUsage {
		if candidate == usage || candidate == x509.ExtKeyUsageAny {
			return true
		}
	}
	return false
}

func sortPairs(items []CertificatePair) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Certificate != items[j].Certificate {
			return items[i].Certificate < items[j].Certificate
		}
		return items[i].PrivateKey < items[j].PrivateKey
	})
}
