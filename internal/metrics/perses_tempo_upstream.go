package metrics

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Compiled mirror of the pinned Perses v0.55.1 datasource model (round-4
// recipe, act-luna-r3.md).
//
// These types transcribe, field for field and tag for tag, the upstream model
// the Perses server itself marshals: v1.Datasource and v1.ProjectMetadata
// (pkg/model/api/v1/{datasource,metadata}.go of the pinned release),
// datasource.Spec, datasource.HTTPDatasourceSpec, the HTTP proxy envelope and
// spec/go/plugin/plugin.go Plugin (github.com/perses/spec v0.3.0). Upstream
// imports these through Go modules this repository cannot require (the release
// needs a newer Go toolchain than go.mod pins), so the mirror lives here.
// perses_tempo_contract.go proves the mirror against the COMMITTED upstream
// source files — every mirrored field name or json tag must appear verbatim
// in the upstream text, and the upstream Datasource struct block must not
// mention apiVersion — so a mirror edited to drift from upstream fails
// closed, and the SHA-256 pins keep the source copies themselves immutable.

// persesUpstreamDatasource is the v1.Datasource REST resource: exactly
// kind/metadata/spec, with no apiVersion field.
type persesUpstreamDatasource struct {
	Kind     string                        `json:"kind"`
	Metadata persesUpstreamProjectMetadata `json:"metadata"`
	Spec     persesUpstreamDatasourceSpec  `json:"spec"`
}

// persesUpstreamProjectMetadata is the upstream ProjectMetadata: the embedded
// Metadata merged inline with the embedded ProjectMetadataWrapper (project),
// exactly as encoding/json flattens anonymous embeds.
type persesUpstreamProjectMetadata struct {
	persesUpstreamMetadata
	persesUpstreamProjectWrapper
}

type persesUpstreamMetadata struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Version   uint64    `json:"version"`
	Tags      []string  `json:"tags,omitempty"`
}

type persesUpstreamProjectWrapper struct {
	Project string `json:"project"`
}

// persesUpstreamDatasourceSpec is the upstream datasource.Spec: display,
// default, plugin.
type persesUpstreamDatasourceSpec struct {
	Display *persesUpstreamDisplay `json:"display,omitempty"`
	Default bool                   `json:"default"`
	Plugin  persesUpstreamPlugin   `json:"plugin"`
}

// persesUpstreamDisplay is the common.Display name/description pair.
type persesUpstreamDisplay struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// persesUpstreamPlugin is spec/go/plugin/plugin.go Plugin: kind, optional
// plugin metadata, and the plugin spec payload (upstream `any`).
type persesUpstreamPlugin struct {
	Kind     string                  `json:"kind"`
	Metadata *persesUpstreamPluginMD `json:"metadata,omitempty"`
	Spec     any                     `json:"spec"`
}

type persesUpstreamPluginMD struct {
	Version  string `json:"version,omitempty"`
	Registry string `json:"registry,omitempty"`
}

// persesUpstreamHTTPDatasourceSpec is spec/go/datasource HTTPDatasourceSpec:
// directUrl XOR proxy. The proxy is the generic envelope
// proxy.Proxy[http.Config]{kind, spec} with the upstream HTTP config fields.
type persesUpstreamHTTPDatasourceSpec struct {
	DirectURL string                   `json:"directUrl,omitempty"`
	Proxy     *persesUpstreamHTTPProxy `json:"proxy,omitempty"`
}

type persesUpstreamHTTPProxy struct {
	Kind string                     `json:"kind"`
	Spec persesUpstreamHTTPProxyCfg `json:"spec"`
}

type persesUpstreamHTTPProxyCfg struct {
	URL              string                          `json:"url"`
	Timeout          persesUpstreamDurationString    `json:"timeout,omitempty"`
	AllowedEndpoints []persesUpstreamAllowedEndpoint `json:"allowedEndpoints,omitempty"`
	Headers          map[string]string               `json:"headers,omitempty"`
	Secret           string                          `json:"secret,omitempty"`
	AllowHeaders     []string                        `json:"allowHeaders,omitempty"`
	DropHeaders      []string                        `json:"dropHeaders,omitempty"`
	OauthPassthrough bool                            `json:"oauthPassthrough,omitempty"`
}

// persesUpstreamTempoTraceQuerySpec is the tempo plugin's query spec: the
// datasource selector and the TraceQL expression (upstream query.cue).
type persesUpstreamTempoTraceQuerySpec struct {
	Datasource persesUpstreamDatasourceSelector `json:"datasource,omitempty"`
	Query      string                           `json:"query"`
	Limit      int                              `json:"limit,omitempty"`
}

// persesUpstreamDurationString and persesUpstreamAllowedEndpoint mirror the
// upstream common.DurationString (a string type) and http.AllowedEndpoint.
type persesUpstreamDurationString string

type persesUpstreamAllowedEndpoint struct {
	EndpointPattern string `json:"endpointPattern"`
	Method          string `json:"method"`
}

type persesUpstreamDatasourceSelector struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

// persesUpstreamQuery is the dashboard panel query model (spec/go/dashboard
// Query): wrapper kind plus a spec that carries the plugin envelope.
type persesUpstreamQuery struct {
	Kind string                   `json:"kind"`
	Spec persesUpstreamQueryOuter `json:"spec"`
}

type persesUpstreamQueryOuter struct {
	Name   string               `json:"name,omitempty"`
	Plugin persesUpstreamPlugin `json:"plugin"`
}

// probe types: the same envelopes with the plugin spec resolved down to the
// tempo plugin's own fields, so the emitted documents' field paths compare
// against what the upstream model can carry.
type persesUpstreamDatasourceProbe struct {
	Kind     string                            `json:"kind"`
	Metadata persesUpstreamProjectMetadata     `json:"metadata"`
	Spec     persesUpstreamDatasourceSpecProbe `json:"spec"`
}

type persesUpstreamDatasourceSpecProbe struct {
	Display *persesUpstreamDisplay         `json:"display,omitempty"`
	Default bool                           `json:"default"`
	Plugin  persesUpstreamPluginDatasource `json:"plugin"`
}

type persesUpstreamPluginDatasource struct {
	Kind     string                           `json:"kind"`
	Metadata *persesUpstreamPluginMD          `json:"metadata,omitempty"`
	Spec     persesUpstreamHTTPDatasourceSpec `json:"spec"`
}

type persesUpstreamQueryProbe struct {
	Kind string                        `json:"kind"`
	Spec persesUpstreamQueryOuterProbe `json:"spec"`
}

type persesUpstreamQueryOuterProbe struct {
	Name   string                    `json:"name,omitempty"`
	Plugin persesUpstreamPluginQuery `json:"plugin"`
}

type persesUpstreamPluginQuery struct {
	Kind     string                            `json:"kind"`
	Metadata *persesUpstreamPluginMD           `json:"metadata,omitempty"`
	Spec     persesUpstreamTempoTraceQuerySpec `json:"spec"`
}

// upstreamDatasourceFieldPaths lists the dotted JSON paths the pinned upstream
// datasource model serializes for a Tempo datasource (resolved exactly the way
// encoding/json resolves the mirrored upstream structs).
func upstreamDatasourceFieldPaths() ([]string, error) {
	return jsonFieldPathsOf(reflect.TypeOf(persesUpstreamDatasourceProbe{}), "")
}

// upstreamTraceQueryFieldPaths lists the dotted JSON paths the pinned upstream
// panel-query model serializes for a TempoTraceQuery.
func upstreamTraceQueryFieldPaths() ([]string, error) {
	return jsonFieldPathsOf(reflect.TypeOf(persesUpstreamQueryProbe{}), "")
}

// upstreamModelSourceEvidence proves that every mirrored field (name or json
// tag) exists verbatim in the COMMITTED upstream model source text, and that
// the upstream Datasource struct block carries no apiVersion field. This is
// what makes the mirror checkable against upstream truth rather than against
// itself.
func upstreamModelSourceEvidence() error {
	mirrors := []struct {
		source string
		typ    reflect.Type
	}{
		{"datasource", reflect.TypeOf(persesUpstreamDatasource{})},
		{"metadata", reflect.TypeOf(persesUpstreamProjectMetadata{})},
		{"spec-datasource", reflect.TypeOf(persesUpstreamHTTPDatasourceSpec{})},
		{"spec-proxy", reflect.TypeOf(persesUpstreamHTTPProxy{})},
		{"spec-proxy-http", reflect.TypeOf(persesUpstreamHTTPProxyCfg{})},
		{"spec-plugin", reflect.TypeOf(persesUpstreamPlugin{})},
		{"spec-dashboard", reflect.TypeOf(persesUpstreamQueryOuterProbe{})},
	}
	sources := map[string]string{
		"datasource":      string(persesTempoUpstreamDatasource),
		"metadata":        string(persesTempoUpstreamMetadata),
		"spec-datasource": string(persesTempoUpstreamSpecDatasource),
		"spec-plugin":     string(persesTempoUpstreamSpecPlugin),
		"spec-dashboard":  string(persesTempoUpstreamSpecDashboard),
		"spec-proxy":      string(persesTempoUpstreamSpecProxy),
		"spec-proxy-http": string(persesTempoUpstreamSpecProxyHTTP),
	}
	for _, mirror := range mirrors {
		text, ok := sources[mirror.source]
		if !ok {
			return &TempoWitnessError{Detail: "upstream model source " + mirror.source + " is missing"}
		}
		for _, field := range reflect.VisibleFields(mirror.typ) {
			if field.Anonymous {
				continue
			}
			declared := false
			tag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			for _, needle := range []string{`json:"` + tag, `json:"` + field.Name} {
				if needle != `json:"` && strings.Contains(text, needle) {
					declared = true
				}
			}
			if !declared {
				return &TempoWitnessError{Detail: fmt.Sprintf("mirror field %s.%s is not declared in upstream %s", mirror.typ.Name(), field.Name, mirror.source)}
			}
		}
	}
	// The REST Datasource resource envelope's own fields: the upstream
	// datasource.go must show kind/metadata/spec tags and no apiVersion tag.
	for _, needle := range []string{`json:"kind"`, `json:"metadata"`, `json:"spec"`} {
		if !strings.Contains(sources["datasource"], needle) {
			return &TempoWitnessError{Detail: "the committed upstream datasource source does not declare " + needle}
		}
	}
	if strings.Contains(sources["datasource"], "apiVersion") {
		return &TempoWitnessError{Detail: "the committed upstream datasource source mentions apiVersion — re-check the copy against the release"}
	}
	return nil
}

// structValueBlock is a helper for the contract file's kind-table check: it
// returns whether the marker line exists in the source text.
func structValueBlock(text, marker string) (string, bool) {
	i := strings.Index(text, marker)
	if i < 0 {
		return "", false
	}
	end := strings.Index(text[i:], "\n")
	if end < 0 {
		return text[i:], true
	}
	return text[i : i+end], true
}

var _ = json.Marshal
