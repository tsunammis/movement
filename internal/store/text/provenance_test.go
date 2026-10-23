//go:build darwin

package text

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
)

func TestInvalidProvenanceDoesNotCommit(t *testing.T) {
	for _, source := range []domain.Source{
		{Reference: "doc:1", CapturedAt: time.Now()},
		{Provider: "docs", CapturedAt: time.Now()},
		{Provider: "docs", Reference: "doc:1"},
		{Provider: "docs", Reference: "doc:1", CapturedAt: time.Now(), Summary: strings.Repeat("é", 32769)},
	} {
		ctx := context.Background()
		repo := New(t.TempDir())
		if _, err := repo.Initialize(ctx, "UTC"); err != nil {
			t.Fatal(err)
		}
		svc := app.Service{Repo: repo}
		before := storeBytes(t, repo.Dir)
		var create app.Create
		req := createRequest("provenance-create")
		if err := json.Unmarshal(req.Data, &create); err != nil {
			t.Fatal(err)
		}
		create.Sources = []domain.Source{source}
		req.Data = encode(create)
		_, err := svc.Execute(ctx, req)
		var de *domain.Error
		if !errors.As(err, &de) || de.Code != "invalid_input" {
			t.Fatalf("invalid source created: %v", err)
		}
		if !reflect.DeepEqual(before, storeBytes(t, repo.Dir)) {
			t.Fatal("rejected create changed records")
		}
		create.Sources = []domain.Source{{Provider: "docs", Reference: "doc:1", CapturedAt: time.Now().UTC(), Summary: "Relevant excerpt"}}
		req.Data = encode(create)
		created := mustExec(t, svc, req) // failed validation must not reserve the operation ID
		before = storeBytes(t, repo.Dir)
		rev := created.Revision
		update := app.Request{Kind: "update", OperationID: "provenance-edit", TaskID: created.TaskID, Actor: "test", ExpectedRevision: &rev, Data: encode(map[string]any{"fields": map[string]any{"sources": []domain.Source{source}}})}
		_, err = svc.Execute(ctx, update)
		if !errors.As(err, &de) || de.Code != "invalid_input" {
			t.Fatalf("invalid edit accepted: %v", err)
		}
		if !reflect.DeepEqual(before, storeBytes(t, repo.Dir)) {
			t.Fatal("rejected edit changed state/history/receipts")
		}
		state, err := svc.Get(ctx, created.TaskID)
		if err != nil || len(state.History) != 1 || state.Task.Sources[0].Summary != "Relevant excerpt" {
			t.Fatalf("lost provenance: %v", err)
		}
	}
}
