package api

import _ "embed"

// OpenAPI is bundled into the binary for runtimes without repository files.
//
//go:embed openapi.yaml
var OpenAPI []byte
