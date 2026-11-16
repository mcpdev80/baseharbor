package telemetry

import (
	"bytes"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"net/url"
	"os"
	"path/filepath"
)

func (d *Driver) verifyRegisteredExternalDelivery() error {
	source := d.registeredExternal
	bound, ok, err := application.ExistingRuntimeOTLPBinding(d.app, d.files)
	if err != nil || !ok || bound.Provider != capability.ProviderExternalOTLP || bound.ContainerEndpoint != d.externalEndpoint || !equivalentExternalHeaders(bound.Headers, d.externalHeaders) {
		return errors.New("registered OTLP delivery differs from prepared receiver or authorization")
	}
	for _, material := range []struct {
		path string
		data []byte
		name string
	}{{bound.CAFile, source.trust, "ca.pem"}, {bound.ClientCertFile, source.certificate, "client-cert.pem"}, {bound.ClientKeyFile, source.key, "client-key.pem"}} {
		if len(material.data) == 0 {
			if material.path != "" {
				return errors.New("registered OTLP delivery inherited unrelated TLS identity")
			}
			continue
		}
		dir := filepath.Join(d.files.Bindings, "telemetry")
		owner, err := os.Lstat(dir)
		if err != nil || !owner.IsDir() || !bhruntime.ProtectedStateOwner(dir, owner) || material.path != filepath.Join(dir, material.name) {
			return errors.New("registered OTLP TLS projection has no canonical protected owner")
		}
		// The file is readable only through its private owner directory or explicit
		// workload mount. It must still be a stable non-writable regular projection.
		current, err := externalprovider.ReadMaterialReference((&url.URL{Scheme: "file", Path: material.path}).String(), false)
		if err != nil {
			return errors.New("registered OTLP delivered TLS material is unverifiable")
		}
		match := bytes.Equal(bytes.TrimSpace(current), bytes.TrimSpace(material.data))
		zeroRegisteredOTLP(current)
		if !match {
			return errors.New("registered OTLP delivered TLS material differs from prepared source")
		}
	}
	if err := application.VerifyOTLPHostDelivery(d.app, d.files); err != nil {
		return err
	}
	env, err := application.ManagedWorkloadEnvironment(d.app, d.files)
	if err != nil || env["OTEL_EXPORTER_OTLP_ENDPOINT"] != d.externalEndpoint || !equivalentExternalHeaders(env["OTEL_EXPORTER_OTLP_HEADERS"], d.externalHeaders) {
		return errors.New("registered OTLP workload delivery is inconsistent")
	}
	return nil
}
