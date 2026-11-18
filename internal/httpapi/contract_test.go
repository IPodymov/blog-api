package httpapi

import (
	"blog-api/api"
	"blog-api/internal/config"
	"blog-api/internal/service"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestRoutesMatchOpenAPI keeps the embedded contract and the router in sync.
func TestRoutesMatchOpenAPI(t *testing.T) {
	var spec struct {
		OpenAPI string                                `json:"openapi"`
		Paths   map[string]map[string]json.RawMessage `json:"paths"`
	}
	if e := json.Unmarshal(api.OpenAPI, &spec); e != nil {
		t.Fatal(e)
	}
	if spec.OpenAPI != "3.1.0" {
		t.Fatalf("openapi version %q", spec.OpenAPI)
	}
	pathParams := regexp.MustCompile(`{([^}]+)}`)
	documented, ids := map[string]bool{}, map[string]bool{}
	for path, ops := range spec.Paths {
		for method, raw := range ops {
			var op struct {
				OperationID string `json:"operationId"`
				Tags        []string
				Security    []json.RawMessage
				Responses   map[string]json.RawMessage
				Parameters  []struct{ Name, In string }
			}
			if e := json.Unmarshal(raw, &op); e != nil {
				t.Fatal(path, method, e)
			}
			if op.OperationID == "" || ids[op.OperationID] {
				t.Errorf("%s %s: missing or duplicate operationId %q", method, path, op.OperationID)
			}
			ids[op.OperationID] = true
			if len(op.Tags) == 0 || op.Security == nil || len(op.Responses) == 0 {
				t.Errorf("%s %s: tags, security and responses are required", method, path)
			}
			want := map[string]bool{}
			for _, m := range pathParams.FindAllStringSubmatch(path, -1) {
				want[m[1]] = true
			}
			got := map[string]bool{}
			for _, p := range op.Parameters {
				if p.In == "path" {
					got[p.Name] = true
				}
			}
			if len(got) != len(want) {
				t.Errorf("%s %s: path parameters %v, want %v", method, path, got, want)
			}
			for name := range want {
				if !got[name] {
					t.Errorf("%s %s: path parameter %q is not documented", method, path, name)
				}
			}
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}
	h := New(&service.Service{}, config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	routed := map[string]bool{}
	e := chi.Walk(h.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routed[method+" "+route] = true
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	for r := range routed {
		if !documented[r] {
			t.Errorf("route %s is not documented in OpenAPI", r)
		}
	}
	for r := range documented {
		if !routed[r] {
			t.Errorf("OpenAPI operation %s has no route", r)
		}
	}
}
