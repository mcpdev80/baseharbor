package traces

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type TempoInstance struct {
	Endpoint   string
	HTTPClient *http.Client
}

type TempoRealization interface {
	Apply(context.Context) (TempoInstance, error)
	Existing(context.Context) (TempoInstance, error)
	VerifyTrace(context.Context, string) error
	Destroy(context.Context) error
}

type runtimeTempoRealization struct {
	runtime   Runtime
	app       application.Manifest
	issuer    serviceaccess.Issuer
	dataDir   string
	namespace string
}

func newRuntimeTempoRealization(runtime Runtime, app application.Manifest, issuer serviceaccess.Issuer, dataDir, namespace string) TempoRealization {
	return &runtimeTempoRealization{
		runtime:   runtime,
		app:       app,
		issuer:    issuer,
		dataDir:   strings.TrimSpace(dataDir),
		namespace: strings.TrimSpace(namespace),
	}
}

func (r *runtimeTempoRealization) Apply(ctx context.Context) (TempoInstance, error) {
	if _, err := ProvisionAt(ctx, r.runtime, r.issuer, r.app, r.dataDir, r.namespace); err != nil {
		return TempoInstance{}, err
	}
	return r.Existing(ctx)
}

func (r *runtimeTempoRealization) Existing(context.Context) (TempoInstance, error) {
	files, _, err := ExistingProviderFilesAt(r.dataDir, r.namespace, r.app)
	if err != nil {
		return TempoInstance{}, err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return TempoInstance{}, err
	}
	client, err := tempoHTTPClient(r.app, files)
	if err != nil {
		return TempoInstance{}, err
	}
	return TempoInstance{Endpoint: endpoint, HTTPClient: client}, nil
}

func (r *runtimeTempoRealization) VerifyTrace(ctx context.Context, traceID string) error {
	return VerifyTraceAt(ctx, r.app, traceID, r.dataDir, r.namespace)
}

func (r *runtimeTempoRealization) Destroy(ctx context.Context) error {
	return DestroyProviderAt(ctx, r.runtime, r.app, filepath.Clean(r.dataDir), r.namespace)
}
