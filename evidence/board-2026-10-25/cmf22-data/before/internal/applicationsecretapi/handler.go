package applicationsecretapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/apierror"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/authorization"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// SecretService is the transport-facing subset of the application secret service.
type SecretService interface {
	List(context.Context, string) ([]applicationsecret.Metadata, error)
	Set(context.Context, string, string, []byte) error
	Delete(context.Context, string, string) error
}

// OwnershipResolver proves that an application belongs to the resolved tenant.
// Implementations must fail closed when ownership is unknown or ambiguous.
type OwnershipResolver interface {
	OwnedByTenant(context.Context, string, string) (bool, error)
}

type Handler struct {
	secrets   SecretService
	dynamic   DynamicSecretService
	ownership OwnershipResolver
	rbac      *authorization.Service
	mux       *http.ServeMux
}

func New(secrets SecretService, ownership OwnershipResolver, rbac *authorization.Service) (*Handler, error) {
	if secrets == nil || ownership == nil || rbac == nil {
		return nil, errors.New("application secret API dependencies are required")
	}
	h := &Handler{secrets: secrets, ownership: ownership, rbac: rbac, mux: http.NewServeMux()}
	if dynamic, ok := secrets.(DynamicSecretService); ok {
		h.dynamic = dynamic
	}
	h.mux.HandleFunc("GET /api/v1/apps/{app}/secrets", h.list)
	h.mux.HandleFunc("PUT /api/v1/apps/{app}/secrets/{name}", h.set)
	h.mux.HandleFunc("DELETE /api/v1/apps/{app}/secrets/{name}", h.delete)
	h.mux.HandleFunc("POST /api/v1/apps/{app}/secret-refs", h.createDynamic)
	h.mux.HandleFunc("POST /api/v1/apps/{app}/secret-refs/resolve", h.readDynamic)
	h.mux.HandleFunc("PUT /api/v1/apps/{app}/secret-refs/resolve", h.rotateDynamic)
	h.mux.HandleFunc("DELETE /api/v1/apps/{app}/secret-refs/resolve", h.deleteDynamic)
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	if err := h.authorize(r.Context(), app, authorization.PermRead); err != nil {
		writeError(w, err)
		return
	}
	items, err := h.secrets.List(r.Context(), app)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Secrets []applicationsecret.Metadata `json:"secrets"`
	}{Secrets: items})
}

func (h *Handler) set(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	name := r.PathValue("name")
	if err := h.authorize(r.Context(), app, authorization.PermUpdate); err != nil {
		writeError(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, (1<<20)+(64<<10))
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Value string `json:"value"`
	}
	if err := decoder.Decode(&request); err != nil {
		writeError(w, apierror.New(apierror.CodeBadRequest, "invalid secret request", 0))
		return
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		writeError(w, apierror.New(apierror.CodeBadRequest, "invalid secret request", 0))
		return
	}
	if request.Value == "" {
		writeError(w, apierror.New(apierror.CodeBadRequest, "secret value must not be empty", 0))
		return
	}
	if len([]byte(request.Value)) > 1<<20 {
		writeError(w, apierror.New(apierror.CodeBadRequest, "secret value exceeds the 1048576-byte limit", 0))
		return
	}
	if err := h.secrets.Set(r.Context(), app, name, []byte(request.Value)); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Name       string `json:"name"`
		Configured bool   `json:"configured"`
	}{Name: name, Configured: true})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	name := r.PathValue("name")
	if err := h.authorize(r.Context(), app, authorization.PermDelete); err != nil {
		writeError(w, err)
		return
	}
	if err := h.secrets.Delete(r.Context(), app, name); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) authorize(ctx context.Context, app string, permission authorization.Permission) *apierror.Error {
	if strings.TrimSpace(app) == "" {
		return apierror.New(apierror.CodeBadRequest, "application is required", 0)
	}
	if _, ok := identity.FromContext(ctx); !ok {
		return apierror.New(apierror.CodeUnauthorized, "", 0)
	}
	tenant, ok := tenancy.FromContext(ctx)
	if !ok || tenant.TenantID == "" {
		return apierror.New(apierror.CodeForbidden, "", 0)
	}
	if err := h.rbac.Authorize(tenant.Roles, permission); err != nil {
		return apierror.Wrap(apierror.CodeForbidden, err)
	}
	owned, err := h.ownership.OwnedByTenant(ctx, tenant.TenantID, app)
	if err != nil || !owned {
		return apierror.New(apierror.CodeForbidden, "", 0)
	}
	return nil
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}

func writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, openbao.ErrApplicationSecretNotFound) {
		writeError(w, apierror.Wrap(apierror.CodeNotFound, err))
		return
	}
	writeError(w, apierror.Wrap(apierror.CodeInternal, err))
}

func writeError(w http.ResponseWriter, err *apierror.Error) {
	writeJSON(w, err.Status, err)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
