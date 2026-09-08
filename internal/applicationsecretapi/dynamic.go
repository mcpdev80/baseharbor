package applicationsecretapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mcpdev80/baseharbor/internal/apierror"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/authorization"
)

// DynamicSecretService is the optional application-facing managed-secret
// extension. The existing named-secret operator contract remains independent.
type DynamicSecretService interface {
	CreateDynamic(context.Context, string, []byte) (applicationsecret.Reference, error)
	ReadDynamic(context.Context, string, applicationsecret.Reference) ([]byte, error)
	RotateDynamic(context.Context, string, applicationsecret.Reference, []byte) error
	DeleteDynamic(context.Context, string, applicationsecret.Reference) error
}

func (h *Handler) createDynamic(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	if err := h.authorize(r.Context(), app, authorization.PermUpdate); err != nil {
		writeError(w, err)
		return
	}
	if h.dynamic == nil {
		writeError(w, apierror.New(apierror.CodeInternal, "dynamic secret service is unavailable", 0))
		return
	}
	value, ok := decodeSecretValueRequest(w, r)
	if !ok {
		return
	}
	ref, err := h.dynamic.CreateDynamic(r.Context(), app, []byte(value))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, struct {
		Reference  string `json:"ref"`
		Configured bool   `json:"configured"`
	}{Reference: ref.String(), Configured: true})
}

func (h *Handler) readDynamic(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	if err := h.authorize(r.Context(), app, authorization.PermRead); err != nil {
		writeError(w, err)
		return
	}
	if h.dynamic == nil {
		writeError(w, apierror.New(apierror.CodeInternal, "dynamic secret service is unavailable", 0))
		return
	}
	ref, ok := decodeReferenceRequest(w, r)
	if !ok {
		return
	}
	value, err := h.dynamic.ReadDynamic(r.Context(), app, ref)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, struct {
		Reference string `json:"ref"`
		Value     string `json:"value"`
	}{Reference: ref.String(), Value: string(value)})
}

func (h *Handler) rotateDynamic(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	if err := h.authorize(r.Context(), app, authorization.PermUpdate); err != nil {
		writeError(w, err)
		return
	}
	if h.dynamic == nil {
		writeError(w, apierror.New(apierror.CodeInternal, "dynamic secret service is unavailable", 0))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, (1<<20)+(64<<10))
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Reference string `json:"ref"`
		Value     string `json:"value"`
	}
	if err := decoder.Decode(&request); err != nil || ensureSingleJSONValue(decoder) != nil || request.Reference == "" || request.Value == "" {
		writeError(w, apierror.New(apierror.CodeBadRequest, "invalid dynamic secret request", 0))
		return
	}
	if len([]byte(request.Value)) > 1<<20 {
		writeError(w, apierror.New(apierror.CodeBadRequest, "secret value exceeds the 1048576-byte limit", 0))
		return
	}
	ref := applicationsecret.Reference(request.Reference)
	if err := h.dynamic.RotateDynamic(r.Context(), app, ref, []byte(request.Value)); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, struct {
		Reference  string `json:"ref"`
		Configured bool   `json:"configured"`
	}{Reference: ref.String(), Configured: true})
}

func (h *Handler) deleteDynamic(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	if err := h.authorize(r.Context(), app, authorization.PermDelete); err != nil {
		writeError(w, err)
		return
	}
	if h.dynamic == nil {
		writeError(w, apierror.New(apierror.CodeInternal, "dynamic secret service is unavailable", 0))
		return
	}
	ref, ok := decodeReferenceRequest(w, r)
	if !ok {
		return
	}
	if err := h.dynamic.DeleteDynamic(r.Context(), app, ref); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func decodeSecretValueRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, (1<<20)+(64<<10))
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Value string `json:"value"`
	}
	if err := decoder.Decode(&request); err != nil || ensureSingleJSONValue(decoder) != nil || request.Value == "" {
		writeError(w, apierror.New(apierror.CodeBadRequest, "invalid secret request", 0))
		return "", false
	}
	if len([]byte(request.Value)) > 1<<20 {
		writeError(w, apierror.New(apierror.CodeBadRequest, "secret value exceeds the 1048576-byte limit", 0))
		return "", false
	}
	return request.Value, true
}

func decodeReferenceRequest(w http.ResponseWriter, r *http.Request) (applicationsecret.Reference, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Reference string `json:"ref"`
	}
	if err := decoder.Decode(&request); err != nil || ensureSingleJSONValue(decoder) != nil || request.Reference == "" {
		writeError(w, apierror.New(apierror.CodeBadRequest, "invalid secret reference request", 0))
		return "", false
	}
	return applicationsecret.Reference(request.Reference), true
}
