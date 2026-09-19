package runtimeexecutor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/objectstorage"
)

const ObjectStorageS3V1 = "object-storage.s3/v1"

type ResourceManager interface {
	Create(ctx context.Context, application, environment, name string) (objectstorage.RuntimeResourceBinding, error)
	Get(ctx context.Context, application, environment, name string) (objectstorage.RuntimeResourceBinding, error)
	Delete(ctx context.Context, application, environment, name string) error
}

type Handler struct {
	resources ResourceManager
}

type ExecuteRequest struct {
	Capability string `json:"capability"`
	Operation  string `json:"operation"`
	Name       string `json:"name"`
}

type ExecuteResponse struct {
	ResourceID string                         `json:"resource_id,omitempty"`
	Binding    *objectstorage.RuntimeResourceBinding `json:"binding,omitempty"`
}

func NewHandler(resources ResourceManager) (*Handler, error) {
	if resources == nil {
		return nil, errors.New("runtime executor resource manager is required")
	}
	return &Handler{resources: resources}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/internal/v1/execute" {
		http.NotFound(w, r)
		return
	}
	application, environment, err := workloadIdentity(r)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "invalid workload identity")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request ExecuteRequest
	if err := decoder.Decode(&request); err != nil || ensureEOF(decoder) != nil {
		writeProblem(w, http.StatusBadRequest, "invalid execute request")
		return
	}
	request.Capability = strings.TrimSpace(request.Capability)
	request.Operation = strings.TrimSpace(request.Operation)
	request.Name = strings.TrimSpace(request.Name)
	if request.Capability != ObjectStorageS3V1 || request.Name == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "unsupported capability request")
		return
	}

	switch request.Operation {
	case "runtime.create":
		binding, err := h.resources.Create(r.Context(), application, environment, request.Name)
		if err != nil {
			writeProblem(w, http.StatusBadGateway, "provider operation failed")
			return
		}
		writeJSON(w, http.StatusOK, ExecuteResponse{ResourceID: binding.ResourceID})
	case "runtime.get":
		binding, err := h.resources.Get(r.Context(), application, environment, request.Name)
		if err != nil {
			writeProblem(w, http.StatusNotFound, "runtime resource not found")
			return
		}
		writeJSON(w, http.StatusOK, ExecuteResponse{ResourceID: binding.ResourceID, Binding: &binding})
	case "runtime.delete":
		if err := h.resources.Delete(r.Context(), application, environment, request.Name); err != nil {
			writeProblem(w, http.StatusBadGateway, "provider operation failed")
			return
		}
		writeJSON(w, http.StatusOK, ExecuteResponse{})
	default:
		writeProblem(w, http.StatusUnprocessableEntity, "unsupported runtime operation")
	}
}

func workloadIdentity(r *http.Request) (string, string, error) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return "", "", errors.New("client certificate is required")
	}
	for _, identity := range r.TLS.PeerCertificates[0].URIs {
		app, environment, ok := parseWorkloadURI(identity)
		if ok {
			return app, environment, nil
		}
	}
	return "", "", errors.New("workload SPIFFE identity is required")
}

func parseWorkloadURI(identity *url.URL) (string, string, bool) {
	if identity == nil || identity.Scheme != "spiffe" || identity.Host != "baseharbor" || identity.RawQuery != "" || identity.Fragment != "" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(identity.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "apps" || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}

func writeProblem(w http.ResponseWriter, status int, title string) {
	writeJSON(w, status, map[string]any{
		"type": "about:blank",
		"title": title,
		"status": status,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
