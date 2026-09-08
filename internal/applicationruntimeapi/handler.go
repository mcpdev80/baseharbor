package applicationruntimeapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/apierror"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/applicationruntimeauth"
	"github.com/mcpdev80/baseharbor/internal/auth"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

type SecretService interface {
	CreateDynamic(context.Context, string, []byte) (applicationsecret.Reference, error)
	ReadDynamic(context.Context, string, applicationsecret.Reference) ([]byte, error)
	RotateDynamic(context.Context, string, applicationsecret.Reference, []byte) error
	DeleteDynamic(context.Context, string, applicationsecret.Reference) error
}

type RuntimeVerifier interface {
	Verify(context.Context, string, string) error
}

type Handler struct {
	secrets  SecretService
	verifier RuntimeVerifier
	mux      *http.ServeMux
}

func New(secrets SecretService, verifier RuntimeVerifier) (*Handler, error) {
	if secrets == nil || verifier == nil {
		return nil, errors.New("application runtime API dependencies are required")
	}
	h := &Handler{secrets: secrets, verifier: verifier, mux: http.NewServeMux()}
	h.mux.HandleFunc("POST /runtime/v1/apps/{app}/secret-refs", h.create)
	h.mux.HandleFunc("POST /runtime/v1/apps/{app}/secret-refs/resolve", h.read)
	h.mux.HandleFunc("PUT /runtime/v1/apps/{app}/secret-refs/resolve", h.rotate)
	h.mux.HandleFunc("DELETE /runtime/v1/apps/{app}/secret-refs/resolve", h.delete)
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) authorize(r *http.Request) *apierror.Error {
	app := strings.TrimSpace(r.PathValue("app"))
	if app == "" {
		return apierror.New(apierror.CodeUnauthorized, "", 0)
	}
	token, err := auth.BearerToken(r.Header.Get("Authorization"))
	if err != nil || h.verifier.Verify(r.Context(), app, token) != nil {
		return apierror.New(apierror.CodeUnauthorized, "", 0)
	}
	return nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if err := h.authorize(r); err != nil {
		writeError(w, err)
		return
	}
	value, ok := decodeValue(w, r)
	if !ok {
		return
	}
	app := r.PathValue("app")
	ref, err := h.secrets.CreateDynamic(r.Context(), app, []byte(value))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusCreated, map[string]any{"ref": ref.String(), "configured": true})
}

func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	if err := h.authorize(r); err != nil {
		writeError(w, err)
		return
	}
	ref, ok := decodeRef(w, r)
	if !ok {
		return
	}
	value, err := h.secrets.ReadDynamic(r.Context(), r.PathValue("app"), ref)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, map[string]string{"ref": ref.String(), "value": string(value)})
}

func (h *Handler) rotate(w http.ResponseWriter, r *http.Request) {
	if err := h.authorize(r); err != nil {
		writeError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, (1<<20)+(64<<10))
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Ref   string `json:"ref"`
		Value string `json:"value"`
	}
	if decoder.Decode(&request) != nil || ensureEOF(decoder) != nil || request.Ref == "" || request.Value == "" || len([]byte(request.Value)) > 1<<20 {
		writeError(w, apierror.New(apierror.CodeBadRequest, "invalid dynamic secret request", 0))
		return
	}
	ref := applicationsecret.Reference(request.Ref)
	if err := h.secrets.RotateDynamic(r.Context(), r.PathValue("app"), ref, []byte(request.Value)); err != nil {
		writeServiceError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, map[string]any{"ref": ref.String(), "configured": true})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.authorize(r); err != nil {
		writeError(w, err)
		return
	}
	ref, ok := decodeRef(w, r)
	if !ok {
		return
	}
	if err := h.secrets.DeleteDynamic(r.Context(), r.PathValue("app"), ref); err != nil {
		writeServiceError(w, err)
		return
	}
	noStore(w)
	w.WriteHeader(http.StatusNoContent)
}

func decodeValue(w http.ResponseWriter, r *http.Request) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, (1<<20)+(64<<10))
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct{ Value string `json:"value"` }
	if decoder.Decode(&request) != nil || ensureEOF(decoder) != nil || request.Value == "" || len([]byte(request.Value)) > 1<<20 {
		writeError(w, apierror.New(apierror.CodeBadRequest, "invalid secret request", 0))
		return "", false
	}
	return request.Value, true
}

func decodeRef(w http.ResponseWriter, r *http.Request) (applicationsecret.Reference, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct{ Ref string `json:"ref"` }
	if decoder.Decode(&request) != nil || ensureEOF(decoder) != nil || request.Ref == "" {
		writeError(w, apierror.New(apierror.CodeBadRequest, "invalid secret reference request", 0))
		return "", false
	}
	return applicationsecret.Reference(request.Ref), true
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	}
	return errors.New("multiple JSON values")
}

func noStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-store") }

func writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, openbao.ErrApplicationSecretNotFound) {
		writeError(w, apierror.Wrap(apierror.CodeNotFound, err))
		return
	}
	writeError(w, apierror.Wrap(apierror.CodeInternal, err))
}

func writeError(w http.ResponseWriter, err *apierror.Error) { writeJSON(w, err.Status, err) }

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

var _ RuntimeVerifier = (*applicationruntimeauth.Verifier)(nil)
