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
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
	"github.com/mcpdev80/baseharbor/internal/runtimeresourceapi"
)

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
	for capability, operations := range authorizer.Capabilities() {
		capabilities[capability] = append([]string(nil), operations...)
	}

	if len(authorizer.Capabilities()) > 0 {
		executor, err := runtimeexecutor.NewClient(runtimeexecutor.ClientConfig{
			URL: cfg.RuntimeExecutorURL,
			CAFile: cfg.RuntimeExecutorCAFile,
			CertFile: cfg.RuntimeExecutorCertFile,
			KeyFile: cfg.RuntimeExecutorKeyFile,
		})
		if err != nil {
			return nil, err
		}
		executors := map[string]runtimeoperation.Executor{}
		for capability, operations := range authorizer.Capabilities() {
			for _, operation := range operations {
				if operation == "runtime.create" || operation == "runtime.delete" {
					executors[capability+"\x00"+operation] = executor
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
		resourceHandler, err := runtimeresourceapi.New(cfg.RuntimeAppName, operations, authorizer, executor)
		if err != nil {
			return nil, err
		}
		protected := requireRuntimeBearer(verifier, cfg.RuntimeAppName, resourceHandler)
		mux.Handle("/runtime/v1/resources", protected)
		mux.Handle("/runtime/v1/resources/", protected)
		mux.Handle("/runtime/v1/operations/", protected)
	}

	mux.Handle("GET /runtime/v1/capabilities", requireRuntimeBearer(verifier, cfg.RuntimeAppName, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		type item struct {
			Capability string   `json:"capability"`
			Operations []string `json:"operations"`
		}
		items := make([]item, 0, len(capabilities))
		for capability, operations := range capabilities {
			sort.Strings(operations)
			items = append(items, item{Capability: capability, Operations: operations})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Capability < items[j].Capability })
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": items})
	})))

	return mux, nil
}

func requireRuntimeBearer(verifier applicationruntimeapi.RuntimeVerifier, app string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.BearerToken(r.Header.Get("Authorization"))
		if err != nil || verifier.Verify(r.Context(), strings.TrimSpace(app), token) != nil {
			w.Header().Set("Content-Type", "application/problem+json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type": "about:blank",
				"title": "unauthorized",
				"status": http.StatusUnauthorized,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
