// Command contracts regenerates reviewable schemas; it never touches user data.
package main

import (
	"encoding/json"
	"go.yaml.in/yaml/v3"
	"movement.local/movement/internal/contract"
	"movement.local/movement/internal/domain"
	"os"
	"path/filepath"
	"reflect"
)

func main() {
	if err := os.MkdirAll("api", 0755); err != nil {
		panic(err)
	}
	for name, v := range map[string]any{"task.schema.json": contract.Document("Movement task schema v1", reflect.TypeFor[domain.Task]()), "history-event.schema.json": contract.Document("Movement history frontmatter v1", reflect.TypeFor[domain.Event]())} {
		b, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			panic(e)
		}
		if e = os.WriteFile(filepath.Join("api", name), append(b, '\n'), 0644); e != nil {
			panic(e)
		}
	}
	b, e := yaml.Marshal(contract.OpenAPI())
	if e != nil {
		panic(e)
	}
	if e = os.WriteFile("api/openapi.yaml", b, 0644); e != nil {
		panic(e)
	}
}
