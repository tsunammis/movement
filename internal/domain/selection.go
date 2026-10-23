package domain

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"time"
)

type FollowUp struct {
	Pending   bool   `json:"pending"`
	Due       bool   `json:"due"`
	LastCheck *Event `json:"last_check,omitempty"`
}

func (t Task) FollowUp(events []Event, asOf time.Time) FollowUp {
	f := FollowUp{Pending: t.FollowUpAt != nil}
	for i := range events {
		ev := events[i]
		if ev.Check == nil {
			continue
		}
		copy := ev
		f.LastCheck = &copy
		if t.FollowUpAt != nil && ev.Check.ContextVersion == t.ContextVersion && !ev.Check.ObservedAt.Before(*t.FollowUpAt) && !ev.Check.ObservedAt.After(asOf) {
			f.Pending = false
		}
	}
	f.Due = f.Pending && t.FollowUpAt != nil && !t.FollowUpAt.After(asOf)
	return f
}

type Query struct {
	From               *time.Time `json:"from,omitempty"`
	To                 *time.Time `json:"to,omitempty"`
	AsOf               *time.Time `json:"as_of,omitempty"`
	View               string     `json:"view,omitempty"`
	Flow               string     `json:"flow,omitempty"`
	Statuses           []string   `json:"statuses,omitempty"`
	Assignee           string     `json:"assignee,omitempty"`
	TimeField          string     `json:"time_field,omitempty"`
	IncludeOverdue     *bool      `json:"include_overdue,omitempty"`
	IncludeCarryover   *bool      `json:"include_carryover,omitempty"`
	IncludeUnscheduled bool       `json:"include_unscheduled,omitempty"`
	UndatedOnly        bool       `json:"undated_only,omitempty"`
	PendingOnly        *bool      `json:"pending_only,omitempty"`
	Limit              int        `json:"limit,omitempty"`
	Cursor             string     `json:"cursor,omitempty"`
}
type Selected struct {
	Task                Task     `json:"task"`
	Reasons             []string `json:"reasons"`
	IsOverdue           bool     `json:"is_overdue"`
	Executable          bool     `json:"executable"`
	NeedsReconciliation bool     `json:"needs_reconciliation"`
	FollowUp            FollowUp `json:"follow_up"`
}
type Page struct {
	Items      []Selected `json:"items"`
	AsOf       time.Time  `json:"as_of"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

func yes(v *bool) bool { return v == nil || *v }
func (q *Query) Normalize(now time.Time) error {
	if q.AsOf == nil {
		n := now.UTC()
		q.AsOf = &n
	}
	if q.View == "" {
		q.View = "planning"
	}
	if q.Flow == "" {
		q.Flow = Actionable
	}
	if q.TimeField == "" {
		q.TimeField = "delivery"
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if !contains([]string{"planning", "execution"}, q.View) || !contains([]string{Actionable, WaitingFor, "all"}, q.Flow) || !contains([]string{"delivery", "follow_up"}, q.TimeField) || q.Limit < 1 || q.Limit > 200 {
		return Err("invalid_input", "invalid query mode or page limit (1–200)")
	}
	if (q.From == nil) != (q.To == nil) || (q.From != nil && !q.From.Before(*q.To)) {
		return Err("invalid_input", "from/to must be an increasing pair")
	}
	for _, status := range q.Statuses {
		if !contains([]string{Candidate, Ready, InProgress, Blocked, SeemsCompleted, Done, Dismissed}, status) {
			return Err("invalid_input", "unknown status filter")
		}
	}
	return nil
}
func Select(tasks []Task, history map[string][]Event, q Query, now time.Time) (Page, error) {
	if err := q.Normalize(now); err != nil {
		return Page{}, err
	}
	// Cursor binds pagination to the same normalized filters/as_of. Changes between pages remain best effort.
	type cursor struct {
		Offset int       `json:"offset"`
		Filter string    `json:"filter"`
		AsOf   time.Time `json:"as_of"`
	}
	var c cursor
	if q.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		if e != nil || json.Unmarshal(b, &c) != nil || c.Offset < 0 {
			return Page{}, Err("invalid_input", "invalid cursor")
		}
		if q.AsOf != nil && !q.AsOf.Equal(c.AsOf) {
			return Page{}, Err("invalid_input", "cursor as_of mismatch")
		}
	}
	normalized := q
	normalized.Cursor = ""
	filter, _ := json.Marshal(normalized)
	if q.Cursor != "" && c.Filter != string(filter) {
		return Page{}, Err("invalid_input", "cursor filters changed")
	}
	page := Page{Items: []Selected{}, AsOf: *q.AsOf}
	all := []Selected{}
	for _, t := range tasks {
		if q.Flow != "all" && q.Flow != t.Flow {
			continue
		}
		if q.Assignee != "" && (t.Delegation == nil || (t.Delegation.Assignee.Reference != q.Assignee && t.Delegation.Assignee.DisplayName != q.Assignee)) {
			continue
		}
		statuses := q.Statuses
		if len(statuses) == 0 {
			statuses = []string{Ready, InProgress, Blocked, SeemsCompleted}
		}
		if !contains(statuses, t.Status) {
			continue
		}
		executable := t.Executable(*q.AsOf)
		if q.View == "execution" && !executable {
			continue
		}
		follow := t.FollowUp(history[t.ID], *q.AsOf)
		if q.TimeField == "follow_up" && yes(q.PendingOnly) && !follow.Pending {
			continue
		}
		overdue := !t.Terminal() && t.DueAt != nil && t.DueAt.Before(*q.AsOf)
		first, second := t.ScheduledFor, t.DueAt
		prefix := ""
		if q.TimeField == "follow_up" {
			first = nil
			second = t.FollowUpAt
			prefix = "follow_up_"
		}
		undated := first == nil && second == nil
		if q.UndatedOnly && !undated {
			continue
		}
		reasons := []string{}
		if q.From == nil {
			if !q.UndatedOnly || undated {
				reasons = append(reasons, "all")
			}
		} else {
			if first != nil && !first.Before(*q.From) && first.Before(*q.To) {
				reasons = append(reasons, "scheduled_in_window")
			}
			if second != nil && !second.Before(*q.From) && second.Before(*q.To) {
				reason := "due_in_window"
				if prefix != "" {
					reason = "follow_up_in_window"
				}
				reasons = append(reasons, reason)
			}
			if second != nil && !t.Terminal() && second.Before(*q.AsOf) && yes(q.IncludeOverdue) {
				reasons = append(reasons, prefix+"overdue")
			}
			if second != nil && second.Before(*q.From) && yes(q.IncludeCarryover) {
				reason := "deadline_carryover"
				if prefix != "" {
					reason = "follow_up_carryover"
				}
				reasons = append(reasons, reason)
			}
			if undated && (q.IncludeUnscheduled || q.UndatedOnly) {
				reasons = append(reasons, prefix+"unscheduled")
			}
		}
		if len(reasons) == 0 {
			continue
		}
		all = append(all, Selected{Task: t, Reasons: reasons, IsOverdue: overdue, Executable: executable, NeedsReconciliation: t.Status == InProgress && t.Claim != nil && !t.ActiveClaim(*q.AsOf), FollowUp: follow})
	}
	rank := map[string]int{"high": 3, "normal": 2, "low": 1}
	earliest := func(t Task) *time.Time {
		if q.TimeField == "follow_up" {
			return t.FollowUpAt
		}
		if t.ScheduledFor == nil {
			return t.DueAt
		}
		if t.DueAt == nil || t.ScheduledFor.Before(*t.DueAt) {
			return t.ScheduledFor
		}
		return t.DueAt
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i].Task, all[j].Task
		if rank[a.Priority] != rank[b.Priority] {
			return rank[a.Priority] > rank[b.Priority]
		}
		x, y := earliest(a), earliest(b)
		if x == nil && y != nil {
			return false
		}
		if x != nil && y == nil {
			return true
		}
		if x != nil && !x.Equal(*y) {
			return x.Before(*y)
		}
		return a.ID < b.ID
	})
	start := c.Offset
	if start > len(all) {
		start = len(all)
	}
	end := start + q.Limit
	if end > len(all) {
		end = len(all)
	}
	page.Items = all[start:end]
	if end < len(all) {
		b, _ := json.Marshal(cursor{end, string(filter), *q.AsOf})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
	return page, nil
}

// DateWindow respects calendar-day lengths across daylight saving changes.
func DateWindow(from, to, zone string) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, time.Time{}, Err("invalid_input", "unknown timezone")
	}
	a, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return a, a, Err("invalid_input", "invalid from date")
	}
	b, err := time.ParseInLocation("2006-01-02", to, loc)
	if err != nil || !a.Before(b) {
		return a, b, Err("invalid_input", "invalid exclusive to date")
	}
	return a.UTC(), b.UTC(), nil
}
