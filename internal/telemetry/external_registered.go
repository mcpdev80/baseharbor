package telemetry

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

type registeredOTLPSource struct {
	registration                externalprovider.Registration
	materials                   []*externalprovider.PreparedMaterial
	trust, certificate, key     []byte
	headers                     string
	inputEndpoint, inputHeaders string
	explicitHeaders             bool
	retainedHeaders             bool
}

func (d *Driver) prepareRegisteredExternalSource() error {
	reg, err := application.ResolveExternalProviderRegistrationAt(d.dataDir, d.app, capability.ProviderExternalOTLP)
	if err != nil {
		return err
	}
	u, err := url.Parse(reg.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("registered OTLP receiver requires credential-free HTTPS")
	}
	input, explicit := os.LookupEnv(ExternalHeadersEnv)
	source := &registeredOTLPSource{registration: reg, inputEndpoint: application.ExternalOTLPEndpoint(), inputHeaders: strings.TrimSpace(input), explicitHeaders: explicit}
	if source.inputEndpoint != "" && strings.TrimRight(source.inputEndpoint, "/") != strings.TrimRight(reg.Endpoint, "/") {
		return errors.New("external OTLP endpoint conflicts with selected registration")
	}
	d.externalEndpoint = reg.Endpoint
	source.headers = source.inputHeaders
	if reg.CredentialRef != "" {
		material, err := externalprovider.PrepareMaterialReference(reg.CredentialRef, true)
		if err != nil {
			return err
		}
		source.materials = append(source.materials, material)
		data := material.Bytes()
		credentials, err := externalprovider.DecodeCredentialMaterial(data)
		zeroRegisteredOTLP(data)
		if err != nil {
			return err
		}
		switch {
		case credentials.Token != "" && credentials.Username == "" && credentials.Password == "":
			source.headers = "Authorization=Bearer " + credentials.Token
		case credentials.Token == "" && credentials.Username != "" && credentials.Password != "" && !strings.Contains(credentials.Username, ":"):
			source.headers = "Authorization=Basic " + base64.StdEncoding.EncodeToString([]byte(credentials.Username+":"+credentials.Password))
		default:
			return errors.New("registered OTLP credential requires one supported token or basic client identity")
		}
		if explicit && !equivalentExternalHeaders(source.inputHeaders, source.headers) {
			return errors.New("external OTLP authorization conflicts with registered credential")
		}
	} else {
		same, err := registeredOTLPReferenceWasBound(d.dataDir, d.app, reg.ID)
		if err != nil {
			return err
		}
		source.retainedHeaders = same && !explicit
		source.headers, err = application.RetainedExternalOTLPAuthorization(d.files, reg.Endpoint, source.inputHeaders, explicit || !same)
		if err != nil {
			return err
		}
	}
	if _, err := parseExternalHeaders(source.headers); err != nil {
		return err
	}
	if reg.Trust.Directory != "" {
		return errors.New("registered OTLP certificate-directory delivery requires explicit supported certificate references")
	}
	capture := func(reference string, private bool) ([]byte, error) {
		if reference == "" {
			return nil, nil
		}
		m, err := externalprovider.PrepareMaterialReference(reference, private)
		if err != nil {
			return nil, err
		}
		source.materials = append(source.materials, m)
		return m.Bytes(), nil
	}
	source.trust, err = capture(reg.Trust.CAReference, false)
	if err != nil {
		return err
	}
	if len(source.trust) > 0 {
		if err := externalprovider.ValidatePublicTrustMaterial(source.trust); err != nil {
			return err
		}
	}
	source.certificate, err = capture(reg.Trust.ClientCertificate, false)
	if err != nil {
		return err
	}
	source.key, err = capture(reg.Trust.ClientKey, true)
	if err != nil {
		return err
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if len(source.trust) > 0 {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(source.trust) {
			return errors.New("registered OTLP trust is invalid")
		}
		config.RootCAs = pool
	}
	if len(source.certificate) > 0 || len(source.key) > 0 {
		if externalprovider.ValidatePublicTrustMaterial(source.certificate) != nil {
			return errors.New("registered OTLP public client certificate is invalid")
		}
		certificate, err := tls.X509KeyPair(source.certificate, source.key)
		if err != nil {
			return errors.New("registered OTLP client identity is invalid")
		}
		config.Certificates = []tls.Certificate{certificate}
	}
	d.registeredExternal = source
	d.externalHeaders = source.headers
	d.externalInput = source.inputHeaders
	d.externalExplicit = explicit
	d.client = &http.Client{Transport: &http.Transport{TLSClientConfig: config}, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return d.revalidateRegisteredExternalSource()
}

func equivalentExternalHeaders(a, b string) bool {
	first, e1 := parseExternalHeaders(a)
	second, e2 := parseExternalHeaders(b)
	return e1 == nil && e2 == nil && reflect.DeepEqual(first, second)
}
func registeredOTLPReferenceWasBound(dataDir string, m application.Manifest, reference string) (bool, error) {
	registry, err := (capability.RegistryStore{Path: filepath.Join(dataDir, "provider-registry.json")}).Load()
	if err != nil {
		return false, err
	}
	for _, b := range registry.Bindings {
		if b.ApplicationID != m.ApplicationID || b.Environment != m.Environment || b.Resource.Provider != capability.ProviderExternalOTLP {
			continue
		}
		for _, i := range registry.Instances {
			if i.ID == b.ProviderInstanceID && i.Reference == reference && i.Ownership == capability.OwnershipExternal {
				return true, nil
			}
		}
	}
	return false, nil
}
func (d *Driver) revalidateRegisteredExternalSource() error {
	source := d.registeredExternal
	if source == nil {
		return nil
	}
	reg, err := application.ResolveExternalProviderRegistrationAt(d.dataDir, d.app, capability.ProviderExternalOTLP)
	if err != nil || !reflect.DeepEqual(reg, source.registration) {
		return errors.New("external OTLP registration changed; replan before delivery")
	}
	input, explicit := os.LookupEnv(ExternalHeadersEnv)
	if application.ExternalOTLPEndpoint() != source.inputEndpoint || strings.TrimSpace(input) != source.inputHeaders || explicit != source.explicitHeaders {
		return errors.New("external OTLP input selection changed; replan before delivery")
	}
	if source.retainedHeaders {
		current, err := application.RetainedExternalOTLPAuthorization(d.files, source.registration.Endpoint, "", false)
		if err != nil || !equivalentExternalHeaders(current, source.headers) {
			return errors.New("retained OTLP authorization changed; replan before delivery")
		}
	}
	for _, material := range source.materials {
		if err := material.Revalidate(); err != nil {
			return err
		}
	}
	return nil
}
func zeroRegisteredOTLP(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
