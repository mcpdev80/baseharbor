package managementaccess

import (
	"net/url"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/contracts"
)

// This file provides the canonical JSON-schema resolution used by the store's
// import boundary. The store validates every imported/exported record against
// the normative management-access/v1 record schema and the outer container
// schema before any write, so the Go import boundary and the packaged schema
// cannot drift. Resolution uses only the packaged registry (no network).

// canonicalSchemas resolves a packaged schema by its registry-relative path
// (e.g. "managementaccess/v1/profile.schema.json") to a Resolved validator.
func canonicalSchemas(rel string) (*jsonschema.Resolved, error) {
	raw, err := contracts.ReadSchema(contracts.Namespace + rel)
	if err != nil {
		return nil, err
	}
	var s jsonschema.Schema
	if err := s.UnmarshalJSON(raw); err != nil {
		return nil, err
	}
	return s.Resolve(&jsonschema.ResolveOptions{Loader: func(uri *url.URL) (*jsonschema.Schema, error) {
		innerRaw, err := contracts.ReadSchema(uri.String())
		if err != nil {
			return nil, err
		}
		var inner jsonschema.Schema
		if err := inner.UnmarshalJSON(innerRaw); err != nil {
			return nil, err
		}
		return &inner, nil
	}})
}

var (
	recordSchemaOnce sync.Once
	recordSchema     *jsonschema.Resolved
	recordSchemaErr  error
)

// canonicalRecordSchema resolves and caches the normative record schema once.
func canonicalRecordSchema() (*jsonschema.Resolved, error) {
	recordSchemaOnce.Do(func() {
		recordSchema, recordSchemaErr = canonicalSchemas(schemaID)
	})
	return recordSchema, recordSchemaErr
}

var (
	containerSchemaOnce sync.Once
	containerSchema     *jsonschema.Resolved
	containerSchemaErr  error
)

// canonicalContainerSchema resolves and caches the outer container schema once.
func canonicalContainerSchema() (*jsonschema.Resolved, error) {
	containerSchemaOnce.Do(func() {
		containerSchema, containerSchemaErr = canonicalSchemas(containerSchemaID)
	})
	return containerSchema, containerSchemaErr
}
