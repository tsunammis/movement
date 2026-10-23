//go:build darwin

// Command examples regenerates synthetic bundles using the production service.
// No external sources or personal task directories are accessed.
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
	store "movement.local/movement/internal/store/text"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func raw(v any) json.RawMessage { b, e := json.Marshal(v); must(e); return b }
func save(path string, b []byte) {
	must(os.MkdirAll(filepath.Dir(path), 0755))
	must(os.WriteFile(path, b, 0644))
}
func copyFile(src, dst string) { b, e := os.ReadFile(src); must(e); save(dst, b) }
func main() {
	for _, name := range []string{"basic-task", "waiting-for-task", "personal-email"} {
		directory := filepath.Join("examples", name)
		inputPath := filepath.Join(directory, "definition.json")
		b, e := os.ReadFile(inputPath)
		if os.IsNotExist(e) {
			original, e := os.ReadFile(filepath.Join(directory, "task.json"))
			must(e)
			var fields map[string]json.RawMessage
			must(json.Unmarshal(original, &fields))
			wanted := map[string]json.RawMessage{}
			typ := reflect.TypeFor[app.Create]()
			for i := 0; i < typ.NumField(); i++ {
				key := typ.Field(i).Tag.Get("json")
				if value, ok := fields[key]; ok {
					wanted[key] = value
				}
			}
			if name == "personal-email" {
				wanted["status"] = raw("ready")
			}
			b, e = json.MarshalIndent(wanted, "", "  ")
			must(e)
			save(inputPath, append(b, '\n'))
		} else {
			must(e)
		}
		var definition app.Create
		must(app.Decode(b, &definition))
		temp, e := os.MkdirTemp("", "movement-fixture-")
		must(e)
		ctx := context.Background()
		repo := store.New(temp)
		_, e = repo.Initialize(ctx, "Europe/Paris")
		must(e)
		now := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
		if name == "personal-email" {
			now = time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
		}
		svc := app.Service{Repo: repo, Now: func() time.Time { return now }}
		created, e := svc.Execute(ctx, app.Request{OperationID: "example-" + name + "-create", Kind: "create", Actor: "example-user", Data: raw(definition)})
		must(e)
		if name == "personal-email" {
			now = time.Date(2026, 10, 23, 13, 0, 0, 0, time.UTC)
			rev := created.Revision
			claim, e := svc.Execute(ctx, app.Request{OperationID: "example-email-claim", Kind: "claim", TaskID: created.TaskID, Actor: "example-assistant", ExpectedRevision: &rev, Data: raw(app.Lease{Seconds: 3600})})
			must(e)
			bundle, e := svc.Get(ctx, created.TaskID)
			must(e)
			task := bundle.Task
			now = now.Add(20 * time.Minute)
			evidence := []domain.Evidence{{Reference: "example://gmail/drafts/kevin-update", ObservedAt: now, Description: "Synthetic unsent draft: recipient, project status, deadline and comment summary verified."}}
			results := []domain.CriterionResult{}
			for _, c := range task.Completion.Criteria {
				results = append(results, domain.CriterionResult{CriterionID: c.ID, Result: "met", Evidence: evidence})
			}
			report := domain.Report{AttemptID: task.Claim.AttemptID, ContextVersion: task.ContextVersion, CompletionVersion: task.Completion.Version, GuidanceVersion: task.ExecutionGuidance.Version, TaskRevisionUsed: task.Revision, ObservedAt: now, Outcome: "completion_proposed", Summary: "Synthetic example: saved an unsent Gmail draft for user review. No real Jira access, Gmail operation or email sending occurred. Confirmation as done is still pending.", CriterionResults: results, Evidence: []domain.Evidence{{Reference: "example://jira/project/status", ObservedAt: now, Description: "Synthetic project status, deadline, risks and comments."}}, Outputs: []domain.Output{{Kind: "email_draft", Provider: "gmail", Reference: "example://gmail/drafts/kevin-update", ObservedAt: now, Description: "Synthetic unsent draft for Kevin."}}}
			rev = task.Revision
			_, e = svc.Execute(ctx, app.Request{OperationID: "example-email-report", Kind: "report", TaskID: task.ID, Actor: "example-assistant", ExpectedRevision: &rev, ClaimToken: claim.ClaimToken, Data: raw(app.ReportInput{Report: report})})
			must(e)
		}
		copyFile(filepath.Join(temp, "tasks", created.TaskID, "task.json"), filepath.Join(directory, "task.json"))
		for _, sub := range []string{"history", "receipts"} {
			dest := filepath.Join(directory, sub)
			old, e := os.ReadDir(dest)
			if e != nil && !os.IsNotExist(e) {
				must(e)
			}
			for _, f := range old {
				if !f.IsDir() {
					must(os.Remove(filepath.Join(dest, f.Name())))
				}
			}
			src := filepath.Join(temp, "receipts")
			if sub == "history" {
				src = filepath.Join(temp, "tasks", created.TaskID, "history")
			}
			entries, e := os.ReadDir(src)
			must(e)
			for _, f := range entries {
				copyFile(filepath.Join(src, f.Name()), filepath.Join(dest, f.Name()))
			}
		}
		must(os.RemoveAll(temp))
	}
}
