package controlplaneruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/applicationruntimeapi"
	"github.com/mcpdev80/baseharbor/internal/auth"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
	"github.com/mcpdev80/baseharbor/internal/runtimeresourceapi"
)

type capabilityExecutorRouter map[string]runtimeoperation.Executor

func (r capabilityExecutorRouter) Execute(ctx context.Context, request runtimeoperation.Request) (runtimeoperation.Result, error) {
	executor := r[strings.TrimSpace(request.Capability)]
	if executor == nil {
		return runtimeoperation.Result{}, errors.New("runtime capability has no executor")
	}
	return executor.Execute(ctx, request)
}

func buildBoundRuntimeHandler(ctx context.Context, cfg Config, secrets applicationruntimeapi.SecretService, verifier applicationruntimeapi.RuntimeVerifier) (http.Handler, error) {
	mux := http.NewServeMux()
	capabilities := map[string][]string{}

	if cfg.RuntimeSecretsEnabled {
		if secrets == nil {
			return nil, errors.New("runtime secrets are enabled but no secret service is configured")
		}
		secretHandler, err := applicationruntimeapi.NewBoundWithoutCapabilities(secrets, verifier, cfg.RuntimeAppName)
		if err != nil {
			return nil, err
		}
		mux.Handle("/runtime/v1/secrets", secretHandler)
		mux.Handle("/runtime/v1/secrets/", secretHandler)
		mux.Handle("/runtime/v1/apps/", secretHandler)
		capabilities["secrets/v1"] = []string{"runtime.create", "runtime.get", "runtime.rotate", "runtime.delete"}
	}

	authorizer, err := runtimeresourceapi.NewStaticAuthorizer(cfg.RuntimeAppName, cfg.RuntimePermissionsFile)
	if err != nil {
		return nil, err
	}
	var serviceVerifier *runtimeresourceapi.ServiceTokenVerifier
	if len(authorizer.Capabilities()) > 0 {
		serviceVerifier, err = runtimeresourceapi.NewServiceTokenVerifier(cfg.RuntimeServiceTokensFile)
		if err != nil {
			return nil, err
		}
	}
	for capability, operations := range authorizer.Capabilities() {
		capabilities[capability] = append([]string(nil), operations...)
	}

	if len(authorizer.Capabilities()) > 0 {
		routes := capabilityExecutorRouter{}
		needsRemoteExecutor := false
		for capabilityName := range authorizer.Capabilities() {
			if capabilityName != metricsprovider.RuntimeCapabilityV1 {
				needsRemoteExecutor = true
				break
			}
		}
		if needsRemoteExecutor {
			executor, err := runtimeexecutor.NewClient(runtimeexecutor.ClientConfig{
				URL:      cfg.RuntimeExecutorURL,
				CAFile:   cfg.RuntimeExecutorCAFile,
				CertFile: cfg.RuntimeExecutorCertFile,
				KeyFile:  cfg.RuntimeExecutorKeyFile,
			})
			if err != nil {
				return nil, err
			}
			for capabilityName := range authorizer.Capabilities() {
				if capabilityName != metricsprovider.RuntimeCapabilityV1 {
					routes[capabilityName] = executor
				}
			}
		}
		if _, ok := authorizer.Capabilities()[metricsprovider.RuntimeCapabilityV1]; ok {
			executor, err := metricsprovider.NewRuntimeSourceExecutor(cfg.RuntimeEnvironment, cfg.RuntimeMetricsTargetsDir)
			if err != nil {
				return nil, err
			}
			routes[metricsprovider.RuntimeCapabilityV1] = executor
		}
		executors := map[string]runtimeoperation.Executor{}
		for capabilityName, allowedOperations := range authorizer.Capabilities() {
			executor := routes[capabilityName]
			if executor == nil {
				return nil, errors.New("runtime capability has no executor")
			}
			for _, operation := range allowedOperations {
				if operation == "runtime.create" || operation == "runtime.delete" {
					executors[capabilityName+"\x00"+operation] = executor
				}
			}
		}
		operations, err := runtimeoperation.New(cfg.RuntimeOperationsDir, executors)
		if err != nil {
			return nil, err
		}
		if err := operations.Resume(ctx); err != nil {
			return nil, err
		}
		resourceHandler, err := runtimeresourceapi.New(cfg.RuntimeAppName, operations, authorizer, routes)
		if err != nil {
			return nil, err
		}
		protected := requireRuntimeServiceBearer(serviceVerifier, resourceHandler)
		mux.Handle("/runtime/v1/resources", protected)
		mux.Handle("/runtime/v1/resources/", protected)
		mux.Handle("/runtime/v1/operations/", protected)
	}

	capabilityHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type item struct {
			Capability string   `json:"capability"`
			Operations []string `json:"operations"`
		}
		visible := capabilities
		if serviceVerifier != nil {
			visible = authorizer.CapabilitiesForService(runtimeresourceapi.RuntimeService(r.Context()))
			if cfg.RuntimeSecretsEnabled {
				visible["secrets/v1"] = []string{"runtime.create", "runtime.get", "runtime.rotate", "runtime.delete"}
			}
		}
		items := make([]item, 0, len(visible))
		for capability, operations := range visible {
			sort.Strings(operations)
			items = append(items, item{Capability: capability, Operations: operations})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Capability < items[j].Capability })
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": items})
	})
	if serviceVerifier != nil {
		mux.Handle("GET /runtime/v1/capabilities", requireRuntimeServiceBearer(serviceVerifier, capabilityHandler))
	} else {
		mux.Handle("GET /runtime/v1/capabilities", requireRuntimeBearer(verifier, cfg.RuntimeAppName, capabilityHandler))
	}

	return mux, nil
}

func requireRuntimeServiceBearer(verifier *runtimeresourceapi.ServiceTokenVerifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.BearerToken(r.Header.Get("Authorization"))
		if err != nil || verifier == nil {
			w.Header().Set("Content-Type", "application/problem+json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type": "about:blank", "title": "unauthorized", "status": http.StatusUnauthorized,
			})
			return
		}
		service, err := verifier.Verify(token)
		if err != nil {
			w.Header().Set("Content-Type", "application/problem+json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type": "about:blank", "title": "unauthorized", "status": http.StatusUnauthorized,
			})
			return
		}
		next.ServeHTTP(w, runtimeresourceapi.WithRuntimeService(r, service))
	})
}

func requireRuntimeBearer(verifier applicationruntimeapi.RuntimeVerifier, app string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.BearerToken(r.Header.Get("Authorization"))
		if err != nil || verifier.Verify(r.Context(), strings.TrimSpace(app), token) != nil {
			w.Header().Set("Content-Type", "application/problem+json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type":   "about:blank",
				"title":  "unauthorized",
				"status": http.StatusUnauthorized,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
