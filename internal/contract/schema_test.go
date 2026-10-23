package contract_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.yaml.in/yaml/v3"
	"movement.local/movement/internal/contract"
	"movement.local/movement/internal/domain"
)

func TestGeneratedContractsAreCurrent(t *testing.T) {
	for name, value := range map[string]any{"task.schema.json": contract.Document("Movement task schema v1", reflect.TypeFor[domain.Task]()), "history-event.schema.json": contract.Document("Movement history frontmatter v1", reflect.TypeFor[domain.Event]())} {
		want, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		want = append(want, '\n')
		got, e := os.ReadFile(filepath.Join("../../api", name))
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(want, got) {
			t.Fatalf("%s is stale; run go run ./cmd/contracts", name)
		}
	}
	want, e := yaml.Marshal(contract.OpenAPI())
	if e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile("../../api/openapi.yaml")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(want, got) {
		t.Fatal("OpenAPI is stale; run go run ./cmd/contracts")
	}
}
func TestTaskFixturesAndMalformedRecords(t *testing.T) {
	for _, name := range []string{"basic-task", "waiting-for-task", "personal-email"} {
		b, e := os.ReadFile(filepath.Join("../../examples", name, "task.json"))
		if e != nil {
			t.Fatal(e)
		}
		if e = contract.ValidateTask(b); e != nil {
			t.Fatalf("%s: %v", name, e)
		}
		var task domain.Task
		if e = json.Unmarshal(b, &task); e != nil {
			t.Fatal(e)
		}
		if e = task.Validate(); e != nil {
			t.Fatal(e)
		}
	}
	raw, e := os.ReadFile("../../examples/basic-task/task.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(map[string]any){func(v map[string]any) { delete(v, "context_version") }, func(v map[string]any) { v["status"] = "magically_done" }, func(v map[string]any) { v["id"] = "not-a-uuid" }, func(v map[string]any) { v["unexpected"] = true }, func(v map[string]any) { v["schema_version"] = 99 }} {
		var value map[string]any
		_ = json.Unmarshal(raw, &value)
		mutate(value)
		b, _ := json.Marshal(value)
		if contract.ValidateTask(b) == nil {
			t.Fatalf("accepted malformed task %s", b)
		}
	}
}
func TestOpenAPICoversAllOperationsAndReferences(t *testing.T) {
	doc := contract.OpenAPI()
	paths := doc["paths"].(map[string]any)
	operations := map[string]bool{}
	for _, p := range paths {
		for _, o := range p.(map[string]any) {
			id := o.(map[string]any)["operationId"].(string)
			if operations[id] {
				t.Fatal("duplicate operation " + id)
			}
			operations[id] = true
		}
	}
	for _, name := range []string{"task_create", "task_list", "task_get", "task_update", "task_context", "task_history", "task_claim", "task_renew", "task_release", "task_report", "task_check_report", "task_observe", "get_config", "create_session", "get_session", "delete_session"} {
		if !operations[name] {
			t.Fatal("missing " + name)
		}
	}
	// Every emitted response reference must resolve locally; no network schemas.
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				if k == "$ref" {
					ref := v.(string)
					const prefix = "#/components/schemas/"
					if len(ref) < len(prefix) || ref[:len(prefix)] != prefix {
						t.Fatal(ref)
					}
					schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
					if schemas[ref[len(prefix):]] == nil {
						t.Fatal("unresolved " + ref)
					}
				} else {
					walk(v)
				}
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(doc)
}
