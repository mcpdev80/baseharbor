// Package controlplaneassets supplies the canonical binary-only runtime recipe.
package controlplaneassets

import _ "embed"

// BinaryDockerfile builds a runtime image from an already compiled Linux CLI.
//
//go:embed Dockerfile.binary
var BinaryDockerfile string
