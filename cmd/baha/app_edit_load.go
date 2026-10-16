package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// appEditLoadReader reads the existing authorized application context for a Target.
type appEditLoadReader func(context.Context, string) (GuidedReadContext, error)

// appEditLoaded preserves the exact input document, including fields not editable
// by the compact wizard. Source must be used when producing an unchanged roundtrip.
type appEditLoaded struct {
	Source      json.RawMessage
	Target      string
	ApplicationID string
	Capabilities []capability.Requirement
	Preferences []application.ProviderPreference
	Complete    bool
}

type appEditLoadError struct{ Kind string }

func (e *appEditLoadError) Error() string { return "app edit load: " + e.Kind }

// appEditLoad never rewrites the source, creates an ID, or mutates a Target.
func appEditLoad(ctx context.Context, target string, source []byte, read appEditLoadReader) (appEditLoaded, error) {
	if target == "" || read == nil || !json.Valid(source) {
		return appEditLoaded{}, &appEditLoadError{Kind: "missing"}
	}
	view, err := read(ctx, target)
	if err != nil {
		var guided *GuidedReadError
		if errors.As(err, &guided) {
			switch guided.Kind {
			case "missing", "ambiguous", "incompatible", "wrong_target", "unverifiable":
				return appEditLoaded{}, &appEditLoadError{Kind: guided.Kind}
			}
		}
		return appEditLoaded{}, &appEditLoadError{Kind: "unverifiable"}
	}
	if view.Target != target || view.Footprint.Target != target {
		return appEditLoaded{}, &appEditLoadError{Kind: "wrong_target"}
	}
	if view.ApplicationID == "" || view.Name == "" {
		return appEditLoaded{}, &appEditLoadError{Kind: "missing"}
	}
	// Deep-copy through JSON: both the source document and nested selections
	// remain detached from the authority's in-memory maps and slices.
	var snapshot struct {
		Capabilities []capability.Requirement `json:"capabilities"`
		Preferences []application.ProviderPreference `json:"preferences"`
	}
	raw, err := json.Marshal(struct {
		Capabilities []capability.Requirement `json:"capabilities"`
		Preferences []application.ProviderPreference `json:"preferences"`
	}{view.Capabilities, view.Preferences})
	if err != nil || json.Unmarshal(raw, &snapshot) != nil {
		return appEditLoaded{}, &appEditLoadError{Kind: "unverifiable"}
	}
	return appEditLoaded{
		Source: append(json.RawMessage(nil), source...),
		Target: target, ApplicationID: view.ApplicationID,
		Capabilities: snapshot.Capabilities, Preferences: snapshot.Preferences,
		Complete: view.Complete,
	}, nil
}

// appEditLoadSource is an exact, read-only roundtrip of the original document.
func appEditLoadSource(loaded appEditLoaded) []byte {
	return bytes.Clone(loaded.Source)
}
