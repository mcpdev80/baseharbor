package runtimeapiv1

import _ "embed"

// OpenAPI contains the canonical BaseHarbor Application Runtime API v1 contract.
//
//go:embed openapi.yaml
var OpenAPI []byte
