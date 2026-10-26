package runtimeapidocs

import (
	"net/http"

	runtimeapiv1 "github.com/mcpdev80/baseharbor/spec/runtime-api/v1"
	"github.com/swaggest/swgui/v5emb"
)

const (
	OpenAPIPath = "/openapi.yaml"
	UIPath      = "/"
)

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+OpenAPIPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(runtimeapiv1.OpenAPI)
	})
	mux.Handle(UIPath, v5emb.New("BaseHarbor Application Runtime API", OpenAPIPath, UIPath))
	return mux
}
