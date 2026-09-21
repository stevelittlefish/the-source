package docs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContractReferences(t *testing.T) {
	data, err := Files.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if spec["openapi"] != "3.1.0" {
		t.Fatal("expected OpenAPI 3.1.0")
	}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				if !strings.HasPrefix(ref, "#/") {
					t.Fatalf("nonlocal reference: %s", ref)
				}
				var target any = spec
				for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
					object, ok := target.(map[string]any)
					if !ok {
						t.Fatalf("invalid reference: %s", ref)
					}
					target, ok = object[part]
					if !ok {
						t.Fatalf("missing reference: %s", ref)
					}
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(spec)
	paths := spec["paths"].(map[string]any)
	ids := map[string]bool{}
	for _, path := range []string{"/health", "/api/v1/books", "/api/v1/books/{id}", "/api/v1/books/{id}/text", "/api/v1/books/languages", "/api/v1/books/random", "/api/v1/books/excerpts/random", "/api/v1/lyrics", "/api/v1/lyrics/{id}", "/api/v1/lyrics/{id}/text", "/api/v1/lyrics/random", "/api/v1/lyrics/excerpts/random", "/api/v1/lyrics/tags", "/api/v1/lyrics/languages"} {
		operations, ok := paths[path].(map[string]any)
		if !ok {
			t.Fatalf("missing %s", path)
		}
		for _, method := range []string{"get", "head"} {
			op, ok := operations[method].(map[string]any)
			if !ok {
				t.Fatalf("missing %s %s", method, path)
			}
			id, ok := op["operationId"].(string)
			if !ok || id == "" || ids[id] {
				t.Fatalf("invalid operationId: %v", op["operationId"])
			}
			ids[id] = true
		}
	}
}
