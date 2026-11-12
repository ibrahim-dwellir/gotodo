// Package docs serves the OpenAPI specification and Swagger UI.
package docs

import (
	_ "embed"
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

//go:embed openapi.yaml
var spec []byte

// SpecPath is where the raw OpenAPI document is served.
const SpecPath = "/swagger/openapi.yaml"

// Register mounts Swagger UI at /swagger/ and the spec at SpecPath.
func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+SpecPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(spec)
	})
	mux.Handle("GET /swagger/", httpSwagger.Handler(
		httpSwagger.URL(SpecPath),
		httpSwagger.PersistAuthorization(true),
	))
}
