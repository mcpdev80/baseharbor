package externalprovider

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

type TrustMode string

const (
	TrustAuto     TrustMode = "auto"
	TrustSystem   TrustMode = "system"
	TrustCustomCA TrustMode = "custom-ca"
	TrustMTLS     TrustMode = "mtls"
)

type Trust struct {
	Mode              TrustMode `json:"mode"`
	CAReference       string    `json:"ca_reference,omitempty"`
	ClientCertificate string    `json:"client_certificate_reference,omitempty"`
	ClientKey         string    `json:"client_key_reference,omitempty"`
	Directory         string    `json:"directory,omitempty"`
}

type Registration struct {
	ID               string              `json:"id"`
	ProviderID       string              `json:"provider_id"`
	ProviderVersion  string              `json:"provider_version,omitempty"`
	ProviderProtocol string              `json:"provider_protocol,omitempty"`
	Provider         capability.Provider `json:"provider"`
	Endpoint         string              `json:"endpoint"`
	CredentialRef    string              `json:"credential_ref,omitempty"`
	Trust            Trust               `json:"trust"`
	Metadata         map[string]string   `json:"metadata,omitempty"`
}

func (r Registration) Validate() error {
	r.ID = strings.TrimSpace(r.ID)
	r.ProviderID = strings.TrimSpace(r.ProviderID)
	r.Endpoint = strings.TrimSpace(r.Endpoint)
	r.CredentialRef = strings.TrimSpace(r.CredentialRef)
	if r.ID == "" {
		return errors.New("external provider id is required")
	}
	if r.ProviderID == "" {
		return errors.New("external provider descriptor id is required")
	}
	if r.Provider.Kind == "" || len(r.Provider.Capabilities) == 0 {
		return errors.New("external provider requires a provider kind and at least one capability")
	}
	if r.Endpoint == "" {
		return errors.New("external provider endpoint is required")
	}
	u, err := url.Parse(r.Endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("external provider endpoint must be an absolute URL")
	}
	if u.User != nil {
		return errors.New("external provider endpoint must not contain credentials")
	}
	if err := r.Trust.Validate(); err != nil {
		return err
	}
	for k, v := range r.Metadata {
		if strings.TrimSpace(k) == "" {
			return errors.New("external provider metadata key is empty")
		}
		lk := strings.ToLower(k)
		if strings.Contains(lk, "password") || strings.Contains(lk, "secret") || strings.Contains(lk, "token") || strings.Contains(lk, "private") {
			return fmt.Errorf("external provider metadata key %q may contain secret material; use credential/trust references", k)
		}
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("external provider metadata %q contains a newline", k)
		}
	}
	return nil
}

func (t Trust) Validate() error {
	mode := t.Mode
	if mode == "" {
		mode = TrustAuto
	}
	hasClientCert := strings.TrimSpace(t.ClientCertificate) != ""
	hasClientKey := strings.TrimSpace(t.ClientKey) != ""
	if hasClientCert != hasClientKey {
		return errors.New("external provider client certificate and private-key references must be provided together")
	}
	switch mode {
	case TrustAuto:
		return nil
	case TrustSystem:
		if t.CAReference != "" || hasClientCert || t.Directory != "" {
			return errors.New("system trust cannot include custom CA, certificate directory or client identity references")
		}
	case TrustCustomCA:
		if strings.TrimSpace(t.CAReference) == "" && strings.TrimSpace(t.Directory) == "" {
			return errors.New("custom-ca trust requires ca_reference or directory")
		}
		if hasClientCert {
			return errors.New("custom-ca trust cannot include mTLS client identity")
		}
	case TrustMTLS:
		if !hasClientCert && strings.TrimSpace(t.Directory) == "" {
			return errors.New("mtls trust requires client certificate/private-key references or a certificate directory")
		}
	default:
		return fmt.Errorf("unsupported external provider trust mode %q", mode)
	}
	return nil
}

func (r Registration) Public() Registration {
	out := r
	out.CredentialRef = strings.TrimSpace(out.CredentialRef)
	out.Trust.ClientKey = referenceOnly(out.Trust.ClientKey)
	out.Trust.ClientCertificate = referenceOnly(out.Trust.ClientCertificate)
	out.Trust.CAReference = referenceOnly(out.Trust.CAReference)
	out.Trust.Directory = cleanReferencePath(out.Trust.Directory)
	if len(out.Metadata) > 0 {
		keys := make([]string, 0, len(out.Metadata))
		for k := range out.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		copy := make(map[string]string, len(keys))
		for _, k := range keys {
			copy[k] = out.Metadata[k]
		}
		out.Metadata = copy
	}
	return out
}

func referenceOnly(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	return filepath.Clean(v)
}

func cleanReferencePath(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	return filepath.Clean(v)
}
