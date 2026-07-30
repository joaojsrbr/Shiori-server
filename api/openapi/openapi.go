// Package openapi embeds and serves the OpenAPI 3.1 specification.
package openapi

import (
	_ "embed"
	"net/http"
)

//go:embed shiori.yaml
var specYAML []byte

// Handler returns an http.HandlerFunc that serves the OpenAPI YAML specification.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		w.Write(specYAML)
	}
}
