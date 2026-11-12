package docs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestSpec_ParsesAndDocumentsEveryRoute guards against YAML syntax slips
// (Swagger UI refuses to render an unparseable spec) and undocumented routes.
func TestSpec_ParsesAndDocumentsEveryRoute(t *testing.T) {
	var doc struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(spec, &doc))
	assert.Equal(t, "3.0.3", doc.OpenAPI)

	want := map[string][]string{
		"/api/health":        {"get"},
		"/api/auth/register": {"post"},
		"/api/auth/login":    {"post"},
		"/api/users/me":      {"get", "put"},
		"/api/todos":         {"get", "post"},
		"/api/todos/{id}":    {"get", "put", "delete"},
	}
	for path, methods := range want {
		ops, ok := doc.Paths[path]
		if !assert.True(t, ok, "path %s missing from spec", path) {
			continue
		}
		for _, m := range methods {
			assert.Contains(t, ops, m, "%s %s missing from spec", m, path)
		}
	}
}
