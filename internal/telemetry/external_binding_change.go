package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

// Revision is private consent input, never a public material fingerprint.
type ExternalOTLPBindingChange struct {
	Changed        bool
	Endpoint       string
	InputsVerified bool
	Revision       string `json:"-"`
}

func InspectRegisteredOTLPBindingChange(app application.Manifest, files application.RuntimeFiles, dataDir string) (ExternalOTLPBindingChange, error) {
	d := NewDriverAt(nil, app, files, nil, dataDir, "")
	change := ExternalOTLPBindingChange{Endpoint: d.externalEndpoint}
	bound, ok, err := application.ExistingRuntimeOTLPBinding(app, files)
	if err != nil {
		return change, err
	}
	change.InputsVerified = d.externalAuthErr == nil && d.registeredExternal != nil
	// No prior delivery means no replacement claim. Recovery still needs the
	// selected private input revision below to invalidate stale consent.
	change.Changed = ok && !change.InputsVerified
	if change.InputsVerified {
		change.Endpoint = d.registeredExternal.registration.Endpoint
		if ok {
			change.Changed = d.verifyRegisteredExternalDelivery() != nil
		}
	}
	raw, _ := json.Marshal(bound)
	for _, material := range []struct {
		path    string
		private bool
	}{{files.ApplicationEnv, true}, {bound.CAFile, false}, {bound.ClientCertFile, false}, {bound.ClientKeyFile, false}} {
		if material.path == "" {
			continue
		}
		data, readErr := externalprovider.ReadMaterialReference((&url.URL{Scheme: "file", Path: material.path}).String(), material.private)
		raw = append(raw, 0)
		if readErr == nil {
			raw = append(raw, data...)
		} else {
			raw = append(raw, "unverifiable-delivery"...)
		}
		zeroRegisteredOTLP(data)
	}

	for _, name := range []string{application.OTLPEndpointEnv, ExternalHeadersEnv, application.ProviderExternalReferenceEnv(d.Descriptor().Kind)} {
		value, present := os.LookupEnv(name)
		raw = append(raw, 0)
		raw = append(raw, name...)
		raw = append(raw, 0)
		raw = append(raw, value...)
		if present {
			raw = append(raw, 1)
		}
	}
	// Include current selected input bytes even when a valid secure file contains
	// malformed credentials. Acknowledgements must not survive another edit.
	reg, resolveErr := application.ResolveExternalProviderRegistrationAt(dataDir, app, d.Descriptor().Kind)
	if resolveErr == nil {
		data, _ := json.Marshal(reg)
		raw = append(raw, 0)
		raw = append(raw, data...)
		for _, input := range []struct {
			ref     string
			private bool
		}{{reg.CredentialRef, true}, {reg.Trust.CAReference, false}, {reg.Trust.ClientCertificate, false}, {reg.Trust.ClientKey, true}} {
			if strings.TrimSpace(input.ref) == "" {
				continue
			}
			data, readErr := externalprovider.ReadMaterialReference(input.ref, input.private)
			raw = append(raw, 0)
			if readErr == nil {
				raw = append(raw, data...)
			} else {
				raw = append(raw, "unverifiable-material"...)
			}
			zeroRegisteredOTLP(data)
		}
	}
	if source := d.registeredExternal; source != nil {
		raw = append(raw, 0)
		raw = append(raw, source.headers...)
	}
	digest := sha256.Sum256(raw)
	zeroRegisteredOTLP(raw)
	change.Revision = hex.EncodeToString(digest[:])
	return change, nil
}
