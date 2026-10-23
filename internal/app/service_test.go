package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"movement.local/movement/internal/domain"
	"strings"
	"sync"
	"testing"
	"time"
)

type memory struct {
	mu       sync.Mutex
	bundles  []Bundle
	receipts map[string]Receipt
}

func clone[T any](v T) T { var x T; b, _ := json.Marshal(v); _ = json.Unmarshal(b, &x); return x }
func (m *memory) Snapshot(context.Context) ([]Bundle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return clone(m.bundles), nil
}
func (m *memory) Transact(_ context.Context, op, digest string, fn func([]Bundle) (Change, error)) (Receipt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.receipts == nil {
		m.receipts = map[string]Receipt{}
	}
	if r, ok := m.receipts[op]; ok {
		if r.Digest != digest {
			return Receipt{}, domain.Err("operation_conflict", "changed")
		}
		return r, nil
	}
	ch, e := fn(clone(m.bundles))
	if e != nil {
		return Receipt{}, e
	}
	if ch.Task != nil {
		found := false
		for i, b := range m.bundles {
			if b.Task.ID == ch.Task.ID {
				m.bundles[i] = Bundle{*ch.Task, append(b.History, *ch.Event)}
				found = true
			}
		}
		if !found {
			m.bundles = append(m.bundles, Bundle{*ch.Task, []domain.Event{*ch.Event}})
		}
	}
	m.receipts[op] = ch.Receipt
	return ch.Receipt, nil
}

var instant = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func setup() (Service, *time.Time) {
	now := instant
	return Service{Repo: &memory{}, Now: func() time.Time { return now }}, &now
}
func request(kind, id, op string, rev int64, data any) Request {
	return Request{Kind: kind, TaskID: id, Actor: "agent", OperationID: op, ExpectedRevision: &rev, Data: marshal(data)}
}
func create(t *testing.T, s Service) Receipt {
	t.Helper()
	r, e := s.Execute(context.Background(), request("create", "", "create", 0, Create{Title: "Draft email", Status: domain.Ready, Context: domain.Context{Objective: "Create an unsent draft"}, Completion: domain.Completion{Version: 1, Criteria: []domain.Criterion{{ID: "draft", Description: "draft exists"}}}, CompletionCheck: &domain.CheckInstructions{Version: 1, Prompt: "Inspect draft", CriterionIDs: []string{"draft"}}}))
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func execute(t *testing.T, s Service, r Request) Receipt {
	t.Helper()
	v, e := s.Execute(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	var e *domain.Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("want %s got %v", want, err)
	}
}
func evidence() []domain.Evidence {
	return []domain.Evidence{{Reference: "example://draft", ObservedAt: instant, Description: "synthetic"}}
}
func report(task domain.Task) domain.Report {
	return domain.Report{ContextVersion: task.ContextVersion, CompletionVersion: task.Completion.Version, CheckVersion: 1, TaskRevisionUsed: task.Revision, ObservedAt: instant, Summary: "Draft verified", Outcome: "met", CriterionResults: []domain.CriterionResult{{CriterionID: "draft", Result: "met", Evidence: evidence()}}}
}
func TestCreateIdempotencyBeforeIDGeneration(t *testing.T) {
	s, _ := setup()
	first := create(t, s)
	again := create(t, s)
	if first != again {
		t.Fatal("creation duplicated")
	}
	_, e := s.Execute(context.Background(), request("create", "", "create", 0, Create{Title: "different"}))
	code(t, e, "operation_conflict")
}
func TestClaimCheckAndConfirm(t *testing.T) {
	s, _ := setup()
	ctx := context.Background()
	created := create(t, s)
	claim := execute(t, s, request("claim", created.TaskID, "claim", 1, Lease{}))
	b, _ := s.Get(ctx, created.TaskID)
	r := request("check", created.TaskID, "check", 2, ReportInput{Report: report(b.Task)})
	r.ClaimToken = claim.ClaimToken
	execute(t, s, r)
	b, _ = s.Get(ctx, created.TaskID)
	if b.Task.Status != domain.SeemsCompleted || b.Task.Claim != nil {
		t.Fatal("successful check did not close claim")
	}
	r = request("renew", created.TaskID, "renew", 3, Lease{})
	r.ClaimToken = claim.ClaimToken
	_, e := s.Execute(ctx, r)
	code(t, e, "claim_conflict")
	execute(t, s, request("update", created.TaskID, "confirm", 3, Update{Action: "complete"}))
	b, _ = s.Get(ctx, created.TaskID)
	if b.Task.Status != domain.Done {
		t.Fatal("not done")
	}
}
func TestLateObservationPreservesCurrentClaim(t *testing.T) {
	s, now := setup()
	ctx := context.Background()
	created := create(t, s)
	first := execute(t, s, request("claim", created.TaskID, "claim-1", 1, Lease{Seconds: 30}))
	b, _ := s.Get(ctx, created.TaskID)
	old := report(b.Task)
	old.AttemptID = b.Task.Claim.AttemptID
	old.Outcome = "completion_proposed"
	old.Outputs = []domain.Output{{Kind: "email_draft", Provider: "gmail", Reference: "example://draft", ObservedAt: instant, Description: "draft"}}
	*now = now.Add(time.Minute)
	second := execute(t, s, request("claim", created.TaskID, "claim-2", 2, Lease{}))
	r := request("report", created.TaskID, "old-report", 3, ReportInput{Report: old})
	r.ClaimToken = first.ClaimToken
	_, e := s.Execute(ctx, r)
	code(t, e, "claim_conflict")
	execute(t, s, request("observe", created.TaskID, "observe", 3, Observation{Report: &old}))
	b, _ = s.Get(ctx, created.TaskID)
	if b.Task.Status != domain.InProgress || b.Task.Claim.TokenHash != hash([]byte(second.ClaimToken)) || b.Task.ContextVersion != 1 {
		t.Fatal("late observation changed business state")
	}
	p, _ := s.Context(ctx, created.TaskID)
	if len(p.Unreconciled) != 1 {
		t.Fatal("lost late evidence")
	}
	fresh := report(b.Task)
	fresh.AttemptID = b.Task.Claim.AttemptID
	fresh.Outcome = "completion_proposed"
	fresh.Reconciles = []string{p.Unreconciled[0].EventID}
	r = request("report", created.TaskID, "reconcile", 4, ReportInput{Report: fresh})
	r.ClaimToken = second.ClaimToken
	execute(t, s, r)
	p, _ = s.Context(ctx, created.TaskID)
	if len(p.Unreconciled) != 0 || p.Task.Status != domain.SeemsCompleted {
		t.Fatal("reconciliation failed")
	}
}
func TestContextVersionAndRevisionConflictRetry(t *testing.T) {
	s, _ := setup()
	ctx := context.Background()
	c := create(t, s)
	r := request("update", c.TaskID, "edit", 0, Update{Fields: map[string]json.RawMessage{"priority": marshal("high")}})
	_, e := s.Execute(ctx, r)
	code(t, e, "revision_conflict")
	r.ExpectedRevision = &c.Revision
	execute(t, s, r)
	b, _ := s.Get(ctx, c.TaskID)
	if b.Task.ContextVersion != 1 {
		t.Fatal("priority changed context")
	}
	execute(t, s, request("update", c.TaskID, "context", 2, Update{Fields: map[string]json.RawMessage{"context": marshal(domain.Context{Objective: "Different instructions"})}}))
	b, _ = s.Get(ctx, c.TaskID)
	if b.Task.ContextVersion != 2 {
		t.Fatal("context change not versioned")
	}
	again := execute(t, s, r)
	if again.Revision != 2 {
		t.Fatal("committed retry did not return original receipt")
	}
}
func TestCheckFollowUpAtomicAndStale(t *testing.T) {
	s, _ := setup()
	ctx := context.Background()
	c := create(t, s)
	b, _ := s.Get(ctx, c.TaskID)
	r := report(b.Task)
	next := instant.Add(time.Hour)
	execute(t, s, request("check", c.TaskID, "check", 1, ReportInput{Report: r, NextFollowUpAt: marshal(next)}))
	p, _ := s.Context(ctx, c.TaskID)
	if p.Task.Status != domain.SeemsCompleted || p.Task.FollowUpAt == nil || !p.FollowUp.Pending || p.FollowUp.Due {
		t.Fatal("check and schedule not atomic")
	}
	_, e := s.Execute(ctx, request("check", c.TaskID, "stale", 1, ReportInput{Report: r, NextFollowUpAt: marshal(nil)}))
	code(t, e, "revision_conflict")
	p, _ = s.Context(ctx, c.TaskID)
	if p.Task.FollowUpAt == nil {
		t.Fatal("stale check cleared follow-up")
	}
}
func TestIngestionPreservesManualDecisions(t *testing.T) {
	s, _ := setup()
	ctx := context.Background()
	data := Create{Title: "Fortnightly", Ingestion: &domain.Ingestion{Producer: "routine", ExternalKey: `["doc","url","fill","2026-10"]`}}
	first := execute(t, s, request("create", "", "scan1", 0, data))
	execute(t, s, request("update", first.TaskID, "dismiss", 1, Update{Action: "dismiss"}))
	data.Title = "Changed by routine"
	again := execute(t, s, request("create", "", "scan2", 0, data))
	if !again.Existing || again.TaskID != first.TaskID {
		t.Fatal("duplicate task")
	}
	b, _ := s.Get(ctx, first.TaskID)
	if b.Task.Status != domain.Dismissed || b.Task.Title != "Fortnightly" || len(b.History) != 2 {
		t.Fatal("ingestion overwrote decision")
	}
}

func TestStrictInput(t *testing.T) {
	for _, raw := range []string{`{"title":"a","title":"b"}`, `{"context":{"objective":"a","objective":"b"}}`, `{"unexpected":1}`, `{} {}`} {
		var data Create
		if Decode([]byte(raw), &data) == nil {
			t.Fatalf("accepted ambiguous input %s", raw)
		}
	}
	s, _ := setup()
	c := create(t, s)
	_, err := s.Execute(context.Background(), request("update", c.TaskID, "null-title", 1, Update{Fields: map[string]json.RawMessage{"title": json.RawMessage(`null`)}}))
	code(t, err, "invalid_input")
}

func TestDelayedAssessmentCannotReplaceNewerEvidence(t *testing.T) {
	s, now := setup()
	ctx := context.Background()
	c := create(t, s)
	b, _ := s.Get(ctx, c.TaskID)
	negative := report(b.Task)
	negative.Outcome = "not_met"
	negative.CriterionResults[0].Result = "not_met"
	execute(t, s, request("check", c.TaskID, "newer-negative", 1, ReportInput{Report: negative}))
	b, _ = s.Get(ctx, c.TaskID)
	delayed := report(b.Task)
	delayed.ObservedAt = instant.Add(-time.Minute)
	_, err := s.Execute(ctx, request("check", c.TaskID, "delayed-positive", 2, ReportInput{Report: delayed}))
	code(t, err, "assessment_conflict")
	delayed.ObservedAt = instant
	_, err = s.Execute(ctx, request("check", c.TaskID, "equal-positive", 2, ReportInput{Report: delayed}))
	code(t, err, "assessment_conflict")
	observed := execute(t, s, request("observe", c.TaskID, "preserve", 2, Observation{Report: &delayed}))
	b, _ = s.Get(ctx, c.TaskID)
	if b.Task.Status != domain.Ready || len(b.History) != 3 {
		t.Fatal("delayed evidence changed state or was lost")
	}
	*now = instant.Add(time.Minute)
	fresh := report(b.Task)
	fresh.ObservedAt = *now
	fresh.Reconciles = []string{observed.EventID}
	execute(t, s, request("check", c.TaskID, "fresh-assessment", 3, ReportInput{Report: fresh}))
	p, _ := s.Context(ctx, c.TaskID)
	if p.Task.Status != domain.SeemsCompleted || p.UnreconciledCount != 0 {
		t.Fatal("fresh reconciliation failed")
	}
	// Receipt lookup must still precede the observation-order guard.
	retried := execute(t, s, request("check", c.TaskID, "newer-negative", 1, ReportInput{Report: negative}))
	if retried.Revision != 2 {
		t.Fatal("retry changed")
	}
}

func TestContextPaginationCannotSkipReconciledObservations(t *testing.T) {
	s, _ := setup()
	ctx := context.Background()
	c := create(t, s)
	for i := 0; i < 23; i++ {
		b, _ := s.Get(ctx, c.TaskID)
		r := report(b.Task)
		execute(t, s, request("observe", c.TaskID, fmt.Sprintf("observe-%d", i), b.Task.Revision, Observation{Report: &r}))
	}
	first, err := s.Context(ctx, c.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Unreconciled) != 10 || first.UnreconciledCount != 23 || first.NextObservationsOffset == nil {
		t.Fatalf("missing pagination: %+v", first)
	}
	seen := map[string]bool{}
	p := first
	for {
		for _, ev := range p.Unreconciled {
			if seen[ev.EventID] {
				t.Fatal("duplicate observation")
			}
			seen[ev.EventID] = true
		}
		if p.NextObservationsOffset == nil {
			break
		}
		p, err = s.ContextPage(ctx, c.TaskID, *p.NextObservationsOffset, 10, &first.Task.Revision)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 23 {
		t.Fatalf("lost observations: %d", len(seen))
	}
	_, err = s.ContextPage(ctx, c.TaskID, 10, 10, nil)
	code(t, err, "invalid_input")
	execute(t, s, request("observe", c.TaskID, "ack", first.Task.Revision, Observation{Acknowledges: []string{first.Unreconciled[0].EventID}, Reason: "already handled"}))
	_, err = s.ContextPage(ctx, c.TaskID, 10, 10, &first.Task.Revision)
	code(t, err, "revision_conflict")
}

func TestContextByteBudgetPreservesReachableEvidence(t *testing.T) {
	s, _ := setup()
	ctx := context.Background()
	c := create(t, s)
	for i := 0; i < 9; i++ {
		b, _ := s.Get(ctx, c.TaskID)
		r := report(b.Task)
		r.Evidence = []domain.Evidence{{Reference: "example://large", ObservedAt: instant, Description: strings.Repeat("x", 600000)}}
		execute(t, s, request("observe", c.TaskID, fmt.Sprintf("large-%d", i), b.Task.Revision, Observation{Report: &r}))
	}
	first, err := s.ContextPage(ctx, c.TaskID, 0, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(marshal(first)) > MaxContextBytes || first.NextObservationsOffset == nil || !first.RecentTruncated {
		t.Fatal("unbounded or silent truncation")
	}
	next, err := s.ContextPage(ctx, c.TaskID, *first.NextObservationsOffset, 50, &first.Task.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Unreconciled)+len(next.Unreconciled) != 9 || next.NextObservationsOffset != nil {
		t.Fatal("evidence unreachable")
	}
}

func TestLateObservationValidatesNestedEvidenceAndHistoricalRevision(t *testing.T) {
	s, _ := setup()
	ctx := context.Background()
	c := create(t, s)
	b, _ := s.Get(ctx, c.TaskID)
	cases := []func(*domain.Report){
		func(r *domain.Report) { r.CriterionResults[0].Evidence[0].ObservedAt = instant.Add(time.Hour) },
		func(r *domain.Report) {
			r.Outputs = []domain.Output{{Kind: "draft", Provider: "gmail", Reference: "example://draft", ObservedAt: instant.Add(time.Hour)}}
		},
		func(r *domain.Report) { r.CompletionVersion = 0 },
	}
	for i, change := range cases {
		r := report(b.Task)
		change(&r)
		_, err := s.Execute(ctx, request("observe", c.TaskID, fmt.Sprintf("bad-observe-%d", i), 1, Observation{Report: &r}))
		code(t, err, "invalid_input")
	}
	execute(t, s, request("update", c.TaskID, "new-context", 1, Update{Fields: map[string]json.RawMessage{"description": marshal("new instructions")}}))
	r := report(b.Task)
	r.TaskRevisionUsed = 2
	_, err := s.Execute(ctx, request("observe", c.TaskID, "wrong-context-revision", 2, Observation{Report: &r}))
	code(t, err, "invalid_input")
}

func TestRetiredCriterionIdentity(t *testing.T) {
	s, _ := setup()
	ctx := context.Background()
	c := create(t, s)
	remove := Update{Fields: map[string]json.RawMessage{"completion": marshal(domain.Completion{Version: 1, Criteria: []domain.Criterion{}}), "completion_check": marshal(nil)}}
	execute(t, s, request("update", c.TaskID, "retire", 1, remove))
	changed := Update{Fields: map[string]json.RawMessage{"completion": marshal(domain.Completion{Version: 2, Criteria: []domain.Criterion{{ID: "draft", Description: "Email has been sent"}}})}}
	_, err := s.Execute(ctx, request("update", c.TaskID, "reuse", 2, changed))
	code(t, err, "invalid_input")
	b, _ := s.Get(ctx, c.TaskID)
	if b.Task.Revision != 2 || len(b.History) != 2 {
		t.Fatal("rejected reuse changed history")
	}
	var retired []domain.Criterion
	if err = json.Unmarshal(b.History[1].Changes["retired_criteria"], &retired); err != nil || len(retired) != 1 || retired[0].Description != "draft exists" {
		t.Fatal("retired identity not retained")
	}
	restore := Update{Fields: map[string]json.RawMessage{"completion": marshal(domain.Completion{Version: 2, Criteria: []domain.Criterion{{ID: "draft", Description: "draft exists"}}})}}
	execute(t, s, request("update", c.TaskID, "restore", 2, restore))
	// Existing criteria can evolve under a new version; a different new criterion needs a fresh ID.
	execute(t, s, request("update", c.TaskID, "edit-active", 3, changed))
	changed.Fields["completion"] = marshal(domain.Completion{Version: 4, Criteria: []domain.Criterion{{ID: "new-criterion", Description: "New requirement"}}})
	execute(t, s, request("update", c.TaskID, "new-id", 4, changed))
}

func TestReportCannotClaimAnUnevaluatedRevision(t *testing.T) {
	s, _ := setup()
	ctx := context.Background()
	c := create(t, s)
	claim := execute(t, s, request("claim", c.TaskID, "claim", 1, Lease{}))
	b, _ := s.Get(ctx, c.TaskID)
	r := report(b.Task)
	r.AttemptID = b.Task.Claim.AttemptID
	r.Outcome = "progress"
	r.TaskRevisionUsed = 1
	req := request("report", c.TaskID, "before-claim", 2, ReportInput{Report: r})
	req.ClaimToken = claim.ClaimToken
	_, err := s.Execute(ctx, req)
	code(t, err, "invalid_input")
	edit := request("update", c.TaskID, "change-context", 2, Update{Fields: map[string]json.RawMessage{"description": marshal("New execution instructions")}})
	edit.ClaimToken = claim.ClaimToken
	execute(t, s, edit)
	b, _ = s.Get(ctx, c.TaskID)
	r.ContextVersion = b.Task.ContextVersion
	r.TaskRevisionUsed = 2
	req = request("report", c.TaskID, "mismatched-revision", 3, ReportInput{Report: r})
	req.ClaimToken = claim.ClaimToken
	_, err = s.Execute(ctx, req)
	code(t, err, "invalid_input")
	r.TaskRevisionUsed = 3
	req = request("report", c.TaskID, "fresh-revision", 3, ReportInput{Report: r})
	req.ClaimToken = claim.ClaimToken
	execute(t, s, req)
}

func TestActiveClaimFlowChangeRequiresReleaseOrOverride(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(fmt.Sprint("override=", override), func(t *testing.T) {
			s, _ := setup()
			c := create(t, s)
			lease := execute(t, s, request("claim", c.TaskID, "flow-claim", 1, Lease{}))
			b, _ := s.Get(context.Background(), c.TaskID)
			fields := map[string]json.RawMessage{"flow": marshal(domain.WaitingFor), "delegation": marshal(domain.Delegation{Assignee: domain.Person{DisplayName: "Nathan"}, DelegatedAt: instant, ExpectedOutcome: "Fill the fortnightly"})}
			change := request("update", c.TaskID, "flow-change", b.Task.Revision, Update{Fields: fields})
			change.ClaimToken = lease.ClaimToken
			_, err := s.Execute(context.Background(), change)
			code(t, err, "claim_conflict")
			unchanged, _ := s.Get(context.Background(), c.TaskID)
			if string(marshal(unchanged)) != string(marshal(b)) {
				t.Fatal("rejected flow change altered state or history")
			}
			if override {
				change.Override = true
			} else {
				release := request("release", c.TaskID, "flow-release", b.Task.Revision, struct{}{})
				release.ClaimToken = lease.ClaimToken
				receipt := execute(t, s, release)
				change.ExpectedRevision = &receipt.Revision
			}
			execute(t, s, change)
			after, _ := s.Get(context.Background(), c.TaskID)
			if after.Task.Flow != domain.WaitingFor || after.Task.Claim != nil {
				t.Fatal("explicit release/override did not permit flow change")
			}
		})
	}
}
