//go:build darwin

package text

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
)

func TestStructuredReportsRecoverWithReceipts(t *testing.T) {
	for _, kind := range []string{"report", "check", "observe"} {
		for _, phase := range []string{"staged", "prepared", "event", "task", "receipt", "cleanup"} {
			t.Run(kind+"/"+phase, func(t *testing.T) {
				dir := t.TempDir()
				ctx := context.Background()
				svc := app.Service{Repo: New(dir)}
				data := app.Create{Title: "Synthetic draft", Status: domain.Ready, Context: domain.Context{Objective: "Create an unsent draft"}, Completion: domain.Completion{Version: 1, Criteria: []domain.Criterion{{ID: "draft", Description: "Draft remains unsent"}}}, CompletionCheck: &domain.CheckInstructions{Version: 1, Prompt: "Inspect the saved draft", CriterionIDs: []string{"draft"}}}
				raw, _ := json.Marshal(data)
				c := mustExec(t, svc, app.Request{OperationID: "create", Kind: "create", Actor: "test", Data: raw})
				token := ""
				if kind == "report" {
					rev := int64(1)
					claim := mustExec(t, svc, app.Request{OperationID: "claim", Kind: "claim", TaskID: c.TaskID, Actor: "test", ExpectedRevision: &rev, Data: json.RawMessage(`{}`)})
					token = claim.ClaimToken
				}
				b, e := svc.Get(ctx, c.TaskID)
				if e != nil {
					t.Fatal(e)
				}
				prior := b.Task.Revision
				now := time.Now().UTC()
				r := domain.Report{ContextVersion: b.Task.ContextVersion, CompletionVersion: 1, CheckVersion: 1, TaskRevisionUsed: prior, ObservedAt: now, Outcome: "met", Summary: "Synthetic draft exists and remains unsent", CriterionResults: []domain.CriterionResult{{CriterionID: "draft", Result: "met", Evidence: []domain.Evidence{{Reference: "example://draft", ObservedAt: now, Description: "Unsent draft"}}}}, Outputs: []domain.Output{{Kind: "email_draft", Provider: "gmail", Reference: "example://draft", ObservedAt: now, Description: "Synthetic unsent artifact"}}}
				if kind == "report" {
					r.AttemptID = b.Task.Claim.AttemptID
					r.Outcome = "completion_proposed"
				}
				if kind == "observe" {
					raw, _ = json.Marshal(app.Observation{Report: &r})
				} else {
					raw, _ = json.Marshal(app.ReportInput{Report: r})
				}
				req := app.Request{OperationID: "result", Kind: kind, TaskID: c.TaskID, Actor: "test", ExpectedRevision: &prior, ClaimToken: token, Data: raw}
				output, e := process(dir, phase, &req)
				if e == nil || !strings.Contains(e.Error(), "killed") {
					t.Fatalf("expected crash: %v %s", e, output)
				}
				restored, e := New(dir).Snapshot(ctx)
				if e != nil {
					t.Fatal(e)
				}
				want := prior + 1
				if phase == "staged" {
					want = prior
				}
				if restored[0].Task.Revision != want || int64(len(restored[0].History)) != want {
					t.Fatal("report revision/history mismatch")
				}
				first := mustExec(t, svc, req)
				retry := mustExec(t, svc, req)
				if first != retry {
					t.Fatal("report retry changed response")
				}
				restored, e = New(dir).Snapshot(ctx)
				if e != nil {
					t.Fatal(e)
				}
				b = restored[0]
				last := b.History[len(b.History)-1]
				saved := last.Report
				if kind == "check" {
					saved = last.Check
				}
				if kind == "observe" {
					saved = last.Observation
				}
				if saved == nil || len(saved.Outputs) != 1 || saved.Outputs[0].Reference != "example://draft" || len(saved.CriterionResults) != 1 || b.Task.Revision != prior+1 {
					t.Fatal("structured evidence lost or duplicated")
				}
				status := domain.SeemsCompleted
				if kind == "observe" {
					status = domain.Ready
				}
				if b.Task.Status != status {
					t.Fatal(fmt.Sprint("unexpected state ", b.Task.Status))
				}
			})
		}
	}
}
