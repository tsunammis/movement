//go:build darwin

package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"movement.local/movement/internal/app"
	store "movement.local/movement/internal/store/text"
)

func TestToolsThroughClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repo := store.New(t.TempDir())
	if _, e := repo.Initialize(ctx, "UTC"); e != nil {
		t.Fatal(e)
	}
	svc := app.Service{Repo: repo}
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	ss, e := New(svc).Connect(ctx, serverTransport, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ss.Close()
	cs, e := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer cs.Close()
	listed, e := cs.ListTools(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(listed.Tools) != 12 {
		t.Fatalf("tools = %d", len(listed.Tools))
	}
	call := func(name string, args any) *sdk.CallToolResult {
		t.Helper()
		r, e := cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	create := map[string]any{"operation_id": "capture-1", "actor": "test-agent", "data": map[string]any{"title": "Review Movement", "status": "ready"}}
	r := call("task_create", create)
	if r.IsError {
		t.Fatalf("create: %+v", r)
	}
	var receipt struct {
		Result app.Receipt `json:"result"`
	}
	b, _ := json.Marshal(r.StructuredContent)
	if e = json.Unmarshal(b, &receipt); e != nil {
		t.Fatal(e)
	}
	id := receipt.Result.TaskID
	if id == "" {
		t.Fatalf("no task ID: %s", b)
	}
	again := call("task_create", create)
	b2, _ := json.Marshal(again.StructuredContent)
	if string(b) != string(b2) {
		t.Fatalf("retry changed receipt %s vs %s", b, b2)
	}
	got, e := svc.Get(ctx, id)
	if e != nil {
		t.Fatal(e)
	}
	if len(got.History) != 1 {
		t.Fatal("retry duplicated history")
	}
	for _, name := range []string{"task_get", "task_context", "task_history"} {
		if r := call(name, map[string]any{"task_id": id}); r.IsError {
			t.Fatalf("%s: %+v", name, r)
		}
	}
	if r := call("task_list", map[string]any{}); r.IsError {
		t.Fatal(r)
	}
	r = call("task_update", map[string]any{"operation_id": "edit-1", "actor": "test-agent", "task_id": id, "expected_revision": 99, "data": map[string]any{"fields": map[string]any{"title": "stale edit"}}})
	if !r.IsError {
		t.Fatal("stale write succeeded")
	}
	b, _ = json.Marshal(r.StructuredContent)
	var failure struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &failure)
	if failure.Error.Code != "revision_conflict" {
		t.Fatalf("error = %s", b)
	}
	// Tool dispatch must use the application's check kind, not the CLI spelling.
	r = call("task_check_report", map[string]any{"operation_id": "check-1", "actor": "test-agent", "task_id": id, "expected_revision": 1, "data": map[string]any{"report": map[string]any{}}})
	b, _ = json.Marshal(r.StructuredContent)
	_ = json.Unmarshal(b, &failure)
	if !r.IsError || failure.Error.Code == "" {
		t.Fatalf("missing structured check validation: %s", b)
	}
}
