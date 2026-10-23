package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func stamp(days int) *time.Time { v := now.AddDate(0, 0, days); return &v }
func fixture() Task {
	t := Task{SchemaVersion: 1, ID: NewID(), Revision: 1, ContextVersion: 1, LastEventID: NewID(), Title: "Write email", Status: Ready, CreatedAt: now, UpdatedAt: now, CreatedBy: "user", Context: Context{Objective: "Create draft"}, Completion: Completion{Version: 1, Criteria: []Criterion{{"draft", "Draft exists"}, {"unsent", "Not sent"}}}}
	t.Normalize()
	return t
}
func TestValidationAndTransitions(t *testing.T) {
	task := fixture()
	if e := task.Validate(); e != nil {
		t.Fatal(e)
	}
	task.Flow = WaitingFor
	if task.Validate() == nil {
		t.Fatal("missing delegation accepted")
	}
	task.Delegation = &Delegation{Assignee: Person{DisplayName: "Nathan"}, DelegatedAt: now, ExpectedOutcome: "report"}
	if e := task.Validate(); e != nil {
		t.Fatal(e)
	}
	task.Flow = Actionable
	task.Status = InProgress
	task.Claim = &Claim{AttemptID: NewID(), TokenHash: "hash", Actor: "agent", ExpiresAt: now.Add(time.Hour)}
	if e := task.Transition("complete"); e != nil {
		t.Fatal(e)
	}
	if task.Status != Done || task.Claim != nil {
		t.Fatal("completion retained claim")
	}
	if task.Transition("complete") == nil {
		t.Fatal("terminal completion accepted")
	}
	if e := task.Transition("reopen"); e != nil {
		t.Fatal(e)
	}
	task.SchemaVersion = 2
	if e := task.Validate(); e == nil {
		t.Fatal("new schema accepted")
	}
}
func TestCriteriaAggregate(t *testing.T) {
	task := fixture()
	evidence := []Evidence{{Reference: "example://draft", ObservedAt: now, Description: "draft"}}
	cases := []struct {
		name    string
		results []CriterionResult
		want    string
		invalid bool
	}{
		{"complete", []CriterionResult{{"draft", "met", evidence}, {"unsent", "met", evidence}}, "met", false},
		{"partial", []CriterionResult{{"draft", "met", evidence}}, "inconclusive", false},
		{"negative", []CriterionResult{{"unsent", "not_met", nil}}, "not_met", false},
		{"missing evidence", []CriterionResult{{"draft", "met", nil}}, "", true},
		{"unknown ID", []CriterionResult{{"other", "unknown", nil}}, "", true},
		{"duplicate", []CriterionResult{{"draft", "unknown", nil}, {"draft", "not_met", nil}}, "", true},
		{"future evidence", []CriterionResult{{"draft", "met", []Evidence{{Reference: "x", ObservedAt: now.Add(time.Second)}}}}, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, e := task.Aggregate(c.results, now)
			if (e != nil) != c.invalid || got != c.want {
				t.Fatalf("got %q %v", got, e)
			}
		})
	}
	task.Completion.Criteria = nil
	if got, _ := task.Aggregate(nil, now); got != "inconclusive" {
		t.Fatal("empty criteria establish completion")
	}
}
func TestContextIgnoresBookkeeping(t *testing.T) {
	a := fixture()
	before := string(a.SemanticContext())
	a.Revision++
	a.FollowUpAt = stamp(3)
	a.Priority = "high"
	if string(a.SemanticContext()) != before {
		t.Fatal("bookkeeping invalidates context")
	}
	a.Context.Constraints = []string{"do not send"}
	if string(a.SemanticContext()) == before {
		t.Fatal("constraints not covered")
	}
}
func TestFollowUpObservedNotReceived(t *testing.T) {
	task := fixture()
	task.FollowUpAt = &now
	report := Report{ContextVersion: 1, ObservedAt: now.Add(-time.Hour)}
	events := []Event{{RecordedAt: now.Add(time.Minute), Check: &report}}
	f := task.FollowUp(events, now.Add(time.Hour))
	if !f.Pending || !f.Due {
		t.Fatal("old observation satisfied later schedule")
	}
	report.ObservedAt = now
	f = task.FollowUp(events, now.Add(time.Hour))
	if f.Pending || f.Due {
		t.Fatal("current observation did not satisfy schedule")
	}
	task.ContextVersion++
	if !task.FollowUp(events, now).Pending {
		t.Fatal("changed context did not invalidate check")
	}
	task.FollowUpAt = stamp(1)
	f = task.FollowUp(nil, now)
	if !f.Pending || f.Due {
		t.Fatal("future schedule must be pending but not due")
	}
}
func TestPlanningAndExecution(t *testing.T) {
	ready := fixture()
	ready.DueAt = stamp(-2)
	blocked := fixture()
	blocked.Status = Blocked
	blocked.DueAt = stamp(-1)
	review := fixture()
	review.Status = SeemsCompleted
	review.DueAt = stamp(1)
	undated := fixture()
	claimed := fixture()
	claimed.Status = InProgress
	claimed.DueAt = stamp(1)
	claimed.Claim = &Claim{AttemptID: NewID(), TokenHash: "x", ExpiresAt: now.Add(time.Hour)}
	tasks := []Task{ready, blocked, review, undated, claimed}
	q := Query{From: stamp(0), To: stamp(7), AsOf: &now}
	p, e := Select(tasks, nil, q, now)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 4 {
		t.Fatalf("planning got %d", len(p.Items))
	}
	q.View = "execution"
	p, e = Select(tasks, nil, q, now)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || p.Items[0].Task.ID != ready.ID {
		t.Fatal("execution included ineligible work")
	}
	q.View = "planning"
	q.IncludeUnscheduled = true
	p, _ = Select(tasks, nil, q, now)
	if len(p.Items) != 5 {
		t.Fatal("explicit undated inclusion failed")
	}
}
func TestCarryoverIsNotOverdue(t *testing.T) {
	task := fixture()
	task.DueAt = stamp(2)
	p, e := Select([]Task{task}, nil, Query{From: stamp(7), To: stamp(14), AsOf: &now}, now)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || p.Items[0].IsOverdue || !contains(p.Items[0].Reasons, "deadline_carryover") || contains(p.Items[0].Reasons, "overdue") {
		t.Fatal("future carryover mislabelled")
	}
}
func TestBoundsDedupeAndPagination(t *testing.T) {
	a := fixture()
	a.DueAt = stamp(0)
	a.ScheduledFor = stamp(0)
	b := fixture()
	b.DueAt = stamp(1)
	c := fixture()
	c.DueAt = stamp(2)
	q := Query{From: stamp(0), To: stamp(2), AsOf: &now, Limit: 1}
	p, e := Select([]Task{a, b, c}, nil, q, now)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || p.NextCursor == "" || len(p.Items[0].Reasons) != 2 {
		t.Fatal("bounds/deduplication failed")
	}
	q.Cursor = p.NextCursor
	p, e = Select([]Task{a, b, c}, nil, q, now)
	if e != nil || len(p.Items) != 1 || p.Items[0].Task.ID != b.ID || p.NextCursor != "" {
		t.Fatalf("second page: %+v %v", p, e)
	}
	q.Flow = "all"
	if _, e = Select([]Task{a, b, c}, nil, q, now); e == nil {
		t.Fatal("cursor allowed changed filter")
	}
}
func TestDaylightSavingWindow(t *testing.T) {
	a, b, e := DateWindow("2026-10-25", "2026-10-26", "Europe/Paris")
	if e != nil {
		t.Fatal(e)
	}
	if b.Sub(a) != 25*time.Hour {
		t.Fatalf("autumn day length %s", b.Sub(a))
	}
	a, b, e = DateWindow("2026-03-29", "2026-03-30", "Europe/Paris")
	if e != nil || b.Sub(a) != 23*time.Hour {
		t.Fatalf("spring day length %s %v", b.Sub(a), e)
	}
}
func TestReportFreshnessAndClaim(t *testing.T) {
	task := fixture()
	task.Status = InProgress
	task.Claim = &Claim{AttemptID: NewID(), TokenHash: "x", ExpiresAt: now.Add(time.Hour)}
	ev := []Evidence{{Reference: "example://draft", ObservedAt: now}}
	report := Report{AttemptID: task.Claim.AttemptID, ContextVersion: 1, CompletionVersion: 1, TaskRevisionUsed: 1, ObservedAt: now, Outcome: "completion_proposed", Summary: "saved", CriterionResults: []CriterionResult{{"draft", "met", ev}, {"unsent", "met", ev}}}
	if e := task.ValidateReport(report, false, now); e != nil {
		t.Fatal(e)
	}
	task.Revision++
	if e := task.ValidateReport(report, false, now); e != nil {
		t.Fatal("bookkeeping revision rejected", e)
	}
	task.ContextVersion++
	if task.ValidateReport(report, false, now) == nil {
		t.Fatal("stale context accepted")
	}
}
func TestStableIngestionKey(t *testing.T) {
	task := fixture()
	key, _ := json.Marshal([]string{"jira", "ABC-1", "prepare-update", "2026-10"})
	task.Ingestion = &Ingestion{Producer: "routine", ExternalKey: string(key)}
	if e := task.Validate(); e != nil {
		t.Fatal(e)
	}
	task.Ingestion.ExternalKey = `["jira", "ABC-1", "prepare-update", "2026-10"]`
	if task.Validate() == nil {
		t.Fatal("noncanonical ingestion key accepted")
	}
}

func TestSourceProvenanceValidation(t *testing.T) {
	valid := Source{Provider: "document", Reference: "opaque:document-reference", CapturedAt: now, Summary: "Relevant source excerpt"}
	for _, tc := range []struct {
		name   string
		change func(*Source)
	}{
		{"provider", func(s *Source) { s.Provider = "  " }},
		{"reference", func(s *Source) { s.Reference = "\t" }},
		{"capture time", func(s *Source) { s.CapturedAt = time.Time{} }},
		{"oversized summary", func(s *Source) { s.Summary = strings.Repeat("x", 65537) }},
		{"unicode byte bound", func(s *Source) { s.Summary = strings.Repeat("é", 32769) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := fixture()
			source := valid
			tc.change(&source)
			task.Sources = []Source{source}
			if task.Validate() == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
	task := fixture()
	valid.Summary = strings.Repeat("é", 32768)
	task.Sources = []Source{valid}
	if err := task.Validate(); err != nil {
		t.Fatal(err)
	}
	// Source identifiers remain opaque data; validating provenance must not fetch them.
	task.Sources[0].Reference = "javascript:untrusted-source-text"
	task.Sources[0].Summary = ""
	if err := task.Validate(); err != nil {
		t.Fatal(err)
	}
}
