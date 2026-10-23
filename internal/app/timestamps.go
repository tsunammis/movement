package app

import (
	"encoding/json"
	"movement.local/movement/internal/domain"
	"time"
)

// Normalize only typed contract timestamps, before hashing and recording changes.
// Omitted fields remain omitted and explicit null remains null.
func normalizeTimestamps(payload any) error {
	switch p := payload.(type) {
	case *Create:
		t := domain.Task{ScheduledFor: p.ScheduledFor, DueAt: p.DueAt, FollowUpAt: p.FollowUpAt, Sources: p.Sources, Delegation: p.Delegation}
		t.Normalize()
		p.ScheduledFor, p.DueAt, p.FollowUpAt = t.ScheduledFor, t.DueAt, t.FollowUpAt
		p.Sources, p.Delegation = t.Sources, t.Delegation
	case *Update:
		for key, raw := range p.Fields {
			var normalized json.RawMessage
			var err error
			switch key {
			case "scheduled_for", "due_at", "follow_up_at":
				normalized, err = normalizeRawTime(raw)
			case "sources":
				var sources []domain.Source
				if err = Decode(raw, &sources); err == nil {
					for i := range sources {
						sources[i].CapturedAt = sources[i].CapturedAt.UTC()
					}
					normalized = marshal(sources)
				}
			case "delegation":
				var delegation *domain.Delegation
				if err = Decode(raw, &delegation); err == nil {
					if delegation != nil {
						delegation.DelegatedAt = delegation.DelegatedAt.UTC()
					}
					normalized = marshal(delegation)
				}
			default:
				continue
			}
			if err != nil {
				return err
			}
			p.Fields[key] = normalized
		}
	case *ReportInput:
		p.Report.NormalizeTimestamps()
		if len(p.NextFollowUpAt) > 0 {
			raw, err := normalizeRawTime(p.NextFollowUpAt)
			if err != nil {
				return err
			}
			p.NextFollowUpAt = raw
		}
	case *Observation:
		if p.Report != nil {
			p.Report.NormalizeTimestamps()
		}
	}
	return nil
}

func normalizeRawTime(raw json.RawMessage) (json.RawMessage, error) {
	var value *time.Time
	if err := Decode(raw, &value); err != nil {
		return nil, err
	}
	if value != nil {
		utc := value.UTC()
		value = &utc
	}
	return marshal(value), nil
}
