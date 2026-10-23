package app

import (
	"context"
	"encoding/json"
	"movement.local/movement/internal/domain"
	"strings"
	"testing"
	"time"
)

func TestTimestampNormalizationAndEquivalentRetries(t *testing.T) {
	s, _ := setup()
	offset := instant.In(time.FixedZone("offset", 7200))
	c := Create{Title: "UTC test", Sources: []domain.Source{{Provider: "test", Reference: "opaque", CapturedAt: offset, Summary: "Keep 2026-10-22T14:00:00+02:00 verbatim"}}, Delegation: &domain.Delegation{DelegatedAt: offset}}
	first := execute(t, s, request("create", "", "utc-create", 0, c))
	c.Sources[0].CapturedAt = instant
	c.Delegation.DelegatedAt = instant
	if again := execute(t, s, request("create", "", "utc-create", 0, c)); again != first {
		t.Fatal("equivalent create did not replay receipt")
	}
	fields := map[string]json.RawMessage{"sources": marshal([]domain.Source{{Provider: "test", Reference: "opaque", CapturedAt: offset, Summary: c.Sources[0].Summary}}), "delegation": marshal(domain.Delegation{DelegatedAt: offset}), "due_at": marshal(offset)}
	updated := execute(t, s, request("update", first.TaskID, "utc-update", 1, Update{Fields: fields}))
	fields["sources"] = marshal(c.Sources)
	fields["delegation"] = marshal(c.Delegation)
	fields["due_at"] = marshal(instant)
	if again := execute(t, s, request("update", first.TaskID, "utc-update", 1, Update{Fields: fields})); again != updated {
		t.Fatal("equivalent update did not replay receipt")
	}
	b, err := s.Get(context.Background(), first.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Task.ContextVersion != 1 || len(b.History) != 2 {
		t.Fatal("offset-only edit changed context or retry duplicated history")
	}
	if b.Task.Sources[0].CapturedAt.Location() != time.UTC || b.Task.Delegation.DelegatedAt.Location() != time.UTC || b.Task.DueAt.Location() != time.UTC {
		t.Fatal("stored task not UTC")
	}
	if !strings.Contains(b.Task.Sources[0].Summary, "+02:00") {
		t.Fatal("prose rewritten")
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(b.History[1].Changes["fields"], &stored); err != nil {
		t.Fatal(err)
	}
	var d domain.Delegation
	if err := json.Unmarshal(stored["delegation"], &d); err != nil || d.DelegatedAt.Location() != time.UTC {
		t.Fatal("history patch not normalized", err)
	}
}

func TestNestedReportTimestampNormalization(t *testing.T) {
	for _, kind := range []string{"check", "report", "observe"} {
		t.Run(kind, func(t *testing.T) {
			offset := instant.In(time.FixedZone("offset", -18000))
			r := domain.Report{ObservedAt: offset, Evidence: []domain.Evidence{{ObservedAt: offset}}, Outputs: []domain.Output{{ObservedAt: offset}}, CriterionResults: []domain.CriterionResult{{Evidence: []domain.Evidence{{ObservedAt: offset}}}}}
			var payload any = &ReportInput{Report: r, NextFollowUpAt: marshal(offset)}
			if kind == "observe" {
				payload = &Observation{Report: &r}
			}
			if err := normalizeTimestamps(payload); err != nil {
				t.Fatal(err)
			}
			if kind != "observe" {
				r = payload.(*ReportInput).Report
			}
			for _, got := range []time.Time{r.ObservedAt, r.Evidence[0].ObservedAt, r.Outputs[0].ObservedAt, r.CriterionResults[0].Evidence[0].ObservedAt} {
				if got.Location() != time.UTC || !got.Equal(instant) {
					t.Fatalf("incorrect normalized time: %v", got)
				}
			}
		})
	}
}

func TestTimestampNormalizationPreservesPatchPresence(t *testing.T) {
	for _, raw := range []string{`{}`, `{"due_at":null}`, `{"sources":null}`, `{"delegation":null}`} {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &fields); err != nil {
			t.Fatal(err)
		}
		p := &Update{Fields: fields}
		if err := normalizeTimestamps(p); err != nil {
			t.Fatal(err)
		}
		if string(marshal(p.Fields)) != raw {
			t.Fatalf("presence changed: %s -> %s", raw, marshal(p.Fields))
		}
	}
}

func TestCheckTimestampRetryAndStoredEvidence(t *testing.T) {
	s, _ := setup()
	c := create(t, s)
	b, _ := s.Get(context.Background(), c.TaskID)
	r := report(b.Task)
	offset := instant.In(time.FixedZone("offset", 7200))
	r.ObservedAt = offset
	r.Evidence = []domain.Evidence{{Reference: "source", ObservedAt: offset}}
	r.Outputs = []domain.Output{{Kind: "draft", Provider: "fake", Reference: "draft:1", ObservedAt: offset}}
	for i := range r.CriterionResults {
		for j := range r.CriterionResults[i].Evidence {
			r.CriterionResults[i].Evidence[j].ObservedAt = offset
		}
	}
	next := offset.Add(time.Hour)
	input := ReportInput{Report: r, NextFollowUpAt: marshal(next)}
	first := execute(t, s, request("check", c.TaskID, "utc-check", 1, input))
	input.Report.NormalizeTimestamps()
	input.NextFollowUpAt = marshal(next.UTC())
	if again := execute(t, s, request("check", c.TaskID, "utc-check", 1, input)); again != first {
		t.Fatal("equivalent check did not replay")
	}
	b, _ = s.Get(context.Background(), c.TaskID)
	if len(b.History) != 2 {
		t.Fatal("duplicate check")
	}
	stored := b.History[1].Check
	for _, got := range []time.Time{stored.ObservedAt, stored.Evidence[0].ObservedAt, stored.Outputs[0].ObservedAt, stored.CriterionResults[0].Evidence[0].ObservedAt, *b.Task.FollowUpAt} {
		if got.Location() != time.UTC {
			t.Fatalf("stored check timestamp not UTC: %v", got)
		}
	}
	// Presence remains semantic even when the already committed revision is stale.
	input.NextFollowUpAt = nil
	_, err := s.Execute(context.Background(), request("check", c.TaskID, "utc-check", 1, input))
	code(t, err, "operation_conflict")
}
