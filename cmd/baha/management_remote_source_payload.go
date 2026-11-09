package main

import (
	"encoding/json"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/development"
)

// remoteApplyPayloadWithSource adds the existing portable Git/OCI source model
// to the authorized apply request. It never serializes the XDG-local worktree
// mapping or local source bytes. The receiving Core must bind actual artifacts.
func remoteApplyPayloadWithSource(payload json.RawMessage, source *development.SourceModel) (json.RawMessage, error) {
	if source == nil {
		return payload, nil
	}
	if err := development.ValidateImmutableRemoteSources(*source); err != nil {
		return nil, fmt.Errorf("remote source is not immutable: %w", err)
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(payload, &request); err != nil {
		return nil, fmt.Errorf("decode remote apply request: %w", err)
	}
	if request == nil {
		return nil, fmt.Errorf("remote apply request must be a JSON object")
	}
	if _, exists := request["source_model"]; exists {
		return nil, fmt.Errorf("remote apply request contains a duplicate source selection")
	}
	pinned, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	request["source_model"] = pinned
	return json.Marshal(request)
}
