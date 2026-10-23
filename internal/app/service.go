// Package app exposes task use cases to CLI, MCP and HTTP adapters.
package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"time"

	"movement.local/movement/internal/domain"
)

type Bundle struct {
	Task    domain.Task    `json:"task"`
	History []domain.Event `json:"history"`
}
type Receipt struct {
	OperationID string `json:"operation_id"`
	Digest      string `json:"input_digest"`
	TaskID      string `json:"task_id"`
	Revision    int64  `json:"revision"`
	EventID     string `json:"event_id"`
	ClaimToken  string `json:"claim_token,omitempty"`
	Existing    bool   `json:"existing,omitempty"`
}
type Change struct {
	Task    *domain.Task
	Event   *domain.Event
	Receipt Receipt
}

// Transact serializes recovery, receipt lookup, reads, callback and commit.
// A matching store-wide receipt returns before the callback is invoked.
type Repository interface {
	Snapshot(context.Context) ([]Bundle, error)
	Transact(context.Context, string, string, func([]Bundle) (Change, error)) (Receipt, error)
}
type Service struct {
	Repo Repository
	Now  func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type Request struct {
	OperationID      string          `json:"operation_id"`
	Kind             string          `json:"kind"`
	TaskID           string          `json:"task_id,omitempty"`
	Actor            string          `json:"actor"`
	ExpectedRevision *int64          `json:"expected_revision,omitempty"`
	ClaimToken       string          `json:"claim_token,omitempty"`
	Override         bool            `json:"override,omitempty"`
	Data             json.RawMessage `json:"data"`
}
type Create struct {
	Title             string                    `json:"title"`
	Description       string                    `json:"description"`
	Status            string                    `json:"status"`
	Flow              string                    `json:"flow"`
	Priority          string                    `json:"priority"`
	Tags              []string                  `json:"tags"`
	ScheduledFor      *time.Time                `json:"scheduled_for"`
	DueAt             *time.Time                `json:"due_at"`
	FollowUpAt        *time.Time                `json:"follow_up_at"`
	Context           domain.Context            `json:"context"`
	Completion        domain.Completion         `json:"completion"`
	ExecutionGuidance *domain.Guidance          `json:"execution_guidance"`
	CompletionCheck   *domain.CheckInstructions `json:"completion_check"`
	Delegation        *domain.Delegation        `json:"delegation"`
	Sources           []domain.Source           `json:"sources"`
	Ingestion         *domain.Ingestion         `json:"ingestion"`
}
type Update struct {
	Action string                     `json:"action,omitempty"`
	Fields map[string]json.RawMessage `json:"fields,omitempty"`
}
type Lease struct {
	Seconds int `json:"seconds"`
}
type ReportInput struct {
	Report         domain.Report   `json:"report"`
	NextFollowUpAt json.RawMessage `json:"next_follow_up_at,omitempty"`
}
type Observation struct {
	Report       *domain.Report `json:"report,omitempty"`
	Acknowledges []string       `json:"acknowledges,omitempty"`
	Reason       string         `json:"reason,omitempty"`
}

func Decode(data []byte, v any) error { return DecodeWithLimit(data, v, 1024*1024) }

// DecodeWithLimit applies the same strict JSON rules to bounded internal records.
// Public request adapters use Decode's fixed 1 MiB limit.
func DecodeWithLimit(data []byte, v any, limit int) error {
	if len(data) > limit {
		return domain.Err("invalid_input", "JSON exceeds byte limit")
	}
	if err := checkJSON(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return domain.Err("invalid_input", err.Error())
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return domain.Err("invalid_input", "expected one JSON object")
	}
	return nil
}

// Reject duplicate keys before typed decoding: last-key-wins is unsafe for receipts.
func checkJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 32 {
			return domain.Err("invalid_input", "JSON nesting exceeds 32")
		}
		token, err := d.Token()
		if err != nil {
			return domain.Err("invalid_input", err.Error())
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return domain.Err("invalid_input", "invalid JSON object")
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return domain.Err("invalid_input", "duplicate JSON key")
				}
				seen[name] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		default:
			return domain.Err("invalid_input", "unexpected JSON delimiter")
		}
		if _, err := d.Token(); err != nil {
			return domain.Err("invalid_input", "unterminated JSON container")
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return domain.Err("invalid_input", "expected one JSON value")
	}
	return nil
}
func hash(data []byte) string       { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func marshal(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func (s Service) Execute(ctx context.Context, req Request) (Receipt, error) {
	if strings.TrimSpace(req.OperationID) == "" || len(req.OperationID) > 200 || strings.TrimSpace(req.Actor) == "" || len(req.Actor) > 200 {
		return Receipt{}, domain.Err("invalid_input", "operation_id and actor required (max 200 bytes)")
	}
	if req.Kind != "create" && !domain.ValidID(req.TaskID) {
		return Receipt{}, domain.Err("invalid_input", "valid task ID required")
	}
	if req.Kind == "create" && req.TaskID != "" {
		return Receipt{}, domain.Err("invalid_input", "creation ID is server generated")
	}
	var payload any
	switch req.Kind {
	case "create":
		payload = &Create{}
	case "update":
		payload = &Update{}
	case "claim", "renew":
		payload = &Lease{}
	case "release":
		payload = &struct{}{}
	case "report", "check":
		payload = &ReportInput{}
	case "observe":
		payload = &Observation{}
	default:
		return Receipt{}, domain.Err("invalid_input", "unknown operation")
	}
	if len(req.Data) == 0 {
		req.Data = json.RawMessage(`{}`)
	}
	if err := Decode(req.Data, payload); err != nil {
		return Receipt{}, err
	}
	if c, ok := payload.(*Create); ok {
		if c.Status == "" {
			c.Status = domain.Candidate
		}
		if c.Flow == "" {
			c.Flow = domain.Actionable
		}
		if c.Priority == "" {
			c.Priority = "normal"
		}
		if c.Completion.Version == 0 {
			c.Completion.Version = 1
		}
		if c.Tags == nil {
			c.Tags = []string{}
		}
		if c.Sources == nil {
			c.Sources = []domain.Source{}
		}
		if c.Context.Constraints == nil {
			c.Context.Constraints = []string{}
		}
		if c.Completion.Criteria == nil {
			c.Completion.Criteria = []domain.Criterion{}
		}
	}
	if l, ok := payload.(*Lease); ok {
		if l.Seconds == 0 {
			l.Seconds = 900
		}
		if l.Seconds < 30 || l.Seconds > 3600 {
			return Receipt{}, domain.Err("invalid_input", "lease must be 30–3600 seconds")
		}
	}
	if err := normalizeTimestamps(payload); err != nil {
		return Receipt{}, err
	}
	digest := hash(marshal(struct {
		Kind, TaskID, Actor string
		Override            bool
		Payload             any
	}{req.Kind, req.TaskID, req.Actor, req.Override, payload}))
	return s.Repo.Transact(ctx, req.OperationID, digest, func(bundles []Bundle) (Change, error) {
		now := s.now()
		var task domain.Task
		var history []domain.Event
		if req.Kind == "create" {
			c := payload.(*Create)
			if c.Status != domain.Candidate && c.Status != domain.Ready {
				return Change{}, domain.Err("invalid_input", "new tasks must be candidate or ready")
			}
			if c.Ingestion != nil {
				for _, b := range bundles {
					if b.Task.Ingestion != nil && *b.Task.Ingestion == *c.Ingestion {
						return Change{Receipt: Receipt{req.OperationID, digest, b.Task.ID, b.Task.Revision, b.Task.LastEventID, "", true}}, nil
					}
				}
			}
			task = domain.Task{SchemaVersion: 1, ID: domain.NewID(), ContextVersion: 1, Title: c.Title, Description: c.Description, Status: c.Status, Flow: c.Flow, Priority: c.Priority, Tags: c.Tags, ScheduledFor: c.ScheduledFor, DueAt: c.DueAt, FollowUpAt: c.FollowUpAt, Context: c.Context, Completion: c.Completion, ExecutionGuidance: c.ExecutionGuidance, CompletionCheck: c.CompletionCheck, Delegation: c.Delegation, Sources: c.Sources, Ingestion: c.Ingestion, CreatedAt: now, UpdatedAt: now, CreatedBy: req.Actor}
		} else {
			for _, b := range bundles {
				if b.Task.ID == req.TaskID {
					task = b.Task
					history = b.History
					break
				}
			}
			if task.ID == "" {
				return Change{}, domain.Err("not_found", "task not found")
			}
			if req.ExpectedRevision == nil || *req.ExpectedRevision != task.Revision {
				return Change{}, domain.Conflict(task.Revision)
			}
		}
		original := task
		before := task.SemanticContext()
		event := domain.Event{SchemaVersion: 1, TaskID: task.ID, EventID: domain.NewID(), Actor: req.Actor, RecordedAt: now, OperationID: req.OperationID, InputDigest: digest, Changes: map[string]json.RawMessage{}}
		token := ""
		authorize := func() error {
			if task.Claim == nil {
				return domain.Err("claim_conflict", "task has no execution claim")
			}
			if !task.ActiveClaim(now) || hash([]byte(req.ClaimToken)) != task.Claim.TokenHash {
				return domain.Err("claim_conflict", "claim expired or token differs")
			}
			return nil
		}
		if req.Kind != "create" && req.Kind != "observe" && task.ActiveClaim(now) && !req.Override {
			if err := authorize(); err != nil {
				return Change{}, err
			}
		}
		if req.Override && req.Kind != "observe" {
			task.Claim = nil
			if task.Status == domain.InProgress {
				task.Status = domain.Ready
			}
			event.Changes["claim_override"] = marshal(true)
		}
		switch req.Kind {
		case "create":
			event.Type = "task_created"
			event.Summary = "Task captured"
		case "update":
			u := payload.(*Update)
			if raw, changesFlow := u.Fields["flow"]; changesFlow && task.ActiveClaim(now) && !req.Override {
				var flow string
				if err := Decode(raw, &flow); err != nil {
					return Change{}, err
				}
				if flow != task.Flow {
					return Change{}, domain.Err("claim_conflict", "release or explicitly override the execution claim before changing flow")
				}
			}
			if err := applyFields(&task, u.Fields); err != nil {
				return Change{}, err
			}
			if err := validateCriterionReuse(task.Completion.Criteria, original.Completion.Criteria, history); err != nil {
				return Change{}, err
			}
			retained := map[string]bool{}
			for _, c := range task.Completion.Criteria {
				retained[c.ID] = true
			}
			retired := []domain.Criterion{}
			for _, c := range original.Completion.Criteria {
				if !retained[c.ID] {
					retired = append(retired, c)
				}
			}
			if len(retired) > 0 {
				event.Changes["retired_criteria"] = marshal(retired)
			}
			if u.Action != "" {
				if err := task.Transition(u.Action); err != nil {
					return Change{}, err
				}
			}
			event.Type = "task_updated"
			event.Summary = "Task updated"
			event.Changes["fields"] = marshal(u.Fields)
			if u.Action != "" {
				event.Changes["action"] = marshal(u.Action)
			}
		case "claim":
			if !task.Executable(now) {
				return Change{}, domain.Err("claim_conflict", "task is not eligible for execution")
			}
			token = domain.NewID() + domain.NewID()
			task.Status = domain.InProgress
			task.Claim = &domain.Claim{AttemptID: domain.NewID(), Actor: req.Actor, TokenHash: hash([]byte(token)), ExpiresAt: now.Add(time.Duration(payload.(*Lease).Seconds) * time.Second)}
			event.Changes["claim_token_hash"] = marshal(task.Claim.TokenHash)
			event.Type = "task_claimed"
			event.AttemptID = task.Claim.AttemptID
			event.Summary = "Execution claimed"
		case "renew":
			if err := authorize(); err != nil {
				return Change{}, err
			}
			if task.Status != domain.InProgress || task.Flow != domain.Actionable {
				return Change{}, domain.Err("claim_conflict", "task is no longer executing")
			}
			task.Claim.ExpiresAt = now.Add(time.Duration(payload.(*Lease).Seconds) * time.Second)
			event.Type = "claim_renewed"
			event.AttemptID = task.Claim.AttemptID
			event.Summary = "Execution lease renewed"
		case "release":
			if err := authorize(); err != nil {
				return Change{}, err
			}
			event.AttemptID = task.Claim.AttemptID
			task.Claim = nil
			task.Status = domain.Ready
			event.Type = "claim_released"
			event.Summary = "Execution released"
		case "report", "check":
			input := payload.(*ReportInput)
			r := input.Report
			isCheck := req.Kind == "check"
			if !isCheck {
				if err := authorize(); err != nil {
					return Change{}, err
				}
			}
			if err := validateEvaluatedRevision(r, history); err != nil {
				return Change{}, err
			}
			if err := task.ValidateReport(r, isCheck, now); err != nil {
				return Change{}, err
			}
			if isCheck || r.Outcome == "completion_proposed" {
				for _, prior := range history {
					assessment := prior.Check
					if assessment == nil && prior.Report != nil && prior.Report.Outcome == "completion_proposed" {
						assessment = prior.Report
					}
					if assessment != nil && assessment.ContextVersion == task.ContextVersion && !r.ObservedAt.After(assessment.ObservedAt) {
						return Change{}, domain.Err("assessment_conflict", "assessment must be newer than accepted evidence; preserve delayed evidence with task_observe, then reevaluate")
					}
				}
			}
			if err := validateReconciles(r.Reconciles, history); err != nil {
				return Change{}, err
			}
			event.Summary = r.Summary
			if isCheck {
				event.Type = "completion_checked"
				event.Check = &r
				if r.Outcome == "met" {
					task.Status = domain.SeemsCompleted
				} else if r.Outcome == "not_met" && task.Status == domain.SeemsCompleted {
					task.Status = domain.Ready
				}
				if len(input.NextFollowUpAt) > 0 {
					var next *time.Time
					if err := Decode(input.NextFollowUpAt, &next); err != nil {
						return Change{}, err
					}
					if next != nil && !next.After(now) {
						return Change{}, domain.Err("invalid_input", "next_follow_up_at must be future or null")
					}
					task.FollowUpAt = next
				}
			} else {
				if len(input.NextFollowUpAt) > 0 {
					return Change{}, domain.Err("invalid_input", "only checks may reschedule follow-up")
				}
				event.Type = "work_reported"
				event.Report = &r
				event.AttemptID = r.AttemptID
				switch r.Outcome {
				case "blocked":
					task.Status = domain.Blocked
				case "completion_proposed":
					task.Status = domain.SeemsCompleted
				}
			}
		case "observe":
			o := payload.(*Observation)
			if (o.Report == nil) == (len(o.Acknowledges) == 0) {
				return Change{}, domain.Err("invalid_input", "provide observation or acknowledgement, not both")
			}
			if o.Report != nil {
				r := *o.Report
				if r.Summary == "" || r.ObservedAt.IsZero() || r.ObservedAt.After(now) || r.TaskRevisionUsed < 1 || r.TaskRevisionUsed > task.Revision || r.ContextVersion < 1 || r.ContextVersion > task.ContextVersion || r.CompletionVersion < 1 || r.GuidanceVersion < 0 {
					return Change{}, domain.Err("invalid_input", "invalid original observation context")
				}
				if err := validateEvaluatedRevision(r, history); err != nil {
					return Change{}, err
				}
				if r.AttemptID == "" && r.CheckVersion < 1 {
					return Change{}, domain.Err("invalid_input", "check observation requires check version")
				}
				if err := domain.ValidateReportArtifacts(r, now); err != nil {
					return Change{}, err
				}
				event.Observation = &r
				event.Summary = r.Summary
				event.Type = "observation_recorded"
			} else {
				if strings.TrimSpace(o.Reason) == "" {
					return Change{}, domain.Err("invalid_input", "acknowledgement needs reason")
				}
				if err := validateReconciles(o.Acknowledges, history); err != nil {
					return Change{}, err
				}
				event.Type = "observation_acknowledged"
				event.Summary = o.Reason
				event.Acknowledges = o.Acknowledges
			}
		}
		task.Normalize()
		if req.Kind != "create" && !bytes.Equal(before, task.SemanticContext()) {
			task.ContextVersion++
			if task.Status == domain.SeemsCompleted {
				task.Status = domain.Ready
			}
		}
		if task.Status != domain.InProgress || task.Flow != domain.Actionable {
			task.Claim = nil
		}
		task.Revision++
		task.LastEventID = event.EventID
		task.UpdatedAt = now
		event.Revision = task.Revision
		event.Sequence = task.Revision
		event.ContextVersion = task.ContextVersion
		if original.Status != task.Status {
			event.Changes["status"] = marshal(map[string]string{"from": original.Status, "to": task.Status})
		}
		if string(marshal(original.Claim)) != string(marshal(task.Claim)) {
			event.Changes["claim"] = marshal(map[string]any{"active": task.Claim != nil})
		}
		if string(marshal(original.FollowUpAt)) != string(marshal(task.FollowUpAt)) {
			event.Changes["follow_up_at"] = marshal(map[string]any{"from": original.FollowUpAt, "to": task.FollowUpAt})
		}
		if err := task.Validate(); err != nil {
			return Change{}, err
		}
		receipt := Receipt{req.OperationID, digest, task.ID, task.Revision, event.EventID, token, false}
		return Change{&task, &event, receipt}, nil
	})
}

// Validate the versions against the actual evaluated revision, not just current
// counters supplied by the caller. Attempts cannot precede their own claim.
func validateEvaluatedRevision(r domain.Report, history []domain.Event) error {
	evaluated, attempt := false, r.AttemptID == ""
	for _, ev := range history {
		if ev.Revision == r.TaskRevisionUsed && ev.ContextVersion == r.ContextVersion {
			evaluated = true
		}
		if ev.Type == "task_claimed" && ev.AttemptID == r.AttemptID && ev.Revision <= r.TaskRevisionUsed {
			attempt = true
		}
	}
	if !evaluated {
		return domain.Err("invalid_input", "report versions do not match evaluated history revision")
	}
	if !attempt {
		return domain.Err("invalid_input", "execution attempt did not exist at evaluated revision")
	}
	return nil
}
func validateCriterionReuse(next, current []domain.Criterion, history []domain.Event) error {
	active := map[string]bool{}
	for _, c := range current {
		active[c.ID] = true
	}
	retired := map[string]string{}
	for _, ev := range history {
		if raw := ev.Changes["retired_criteria"]; len(raw) > 0 {
			var entries []domain.Criterion
			if err := Decode(raw, &entries); err != nil {
				return domain.Err("storage_error", "invalid retired criterion history")
			}
			for _, c := range entries {
				retired[c.ID] = c.Description
			}
		}
	}
	for _, c := range next {
		if prior, exists := retired[c.ID]; exists && !active[c.ID] && prior != c.Description {
			return domain.Err("invalid_input", "retired criterion ID cannot be reused with a different description; use a new ID")
		}
	}
	return nil
}
func validateReconciles(ids []string, history []domain.Event) error {
	known := map[string]bool{}
	for _, ev := range history {
		if ev.Observation != nil {
			known[ev.EventID] = true
		}
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !known[id] || seen[id] {
			return domain.Err("invalid_input", "unknown/duplicate observation reference")
		}
		seen[id] = true
	}
	return nil
}
func applyFields(t *domain.Task, fields map[string]json.RawMessage) error {
	for field, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) && field != "scheduled_for" && field != "due_at" && field != "follow_up_at" && field != "delegation" && field != "execution_guidance" && field != "completion_check" {
			return domain.Err("invalid_input", "field cannot be null: "+field)
		}
		var target any
		switch field {
		case "title":
			target = &t.Title
		case "description":
			target = &t.Description
		case "flow":
			target = &t.Flow
		case "priority":
			target = &t.Priority
		case "tags":
			target = &t.Tags
		case "scheduled_for":
			target = &t.ScheduledFor
		case "due_at":
			target = &t.DueAt
		case "follow_up_at":
			target = &t.FollowUpAt
		case "context":
			target = &t.Context
		case "sources":
			target = &t.Sources
		case "delegation":
			target = &t.Delegation
		case "completion":
			target = &t.Completion
		case "execution_guidance":
			target = &t.ExecutionGuidance
		case "completion_check":
			target = &t.CompletionCheck
		default:
			return domain.Err("invalid_input", "field is not editable: "+field)
		}
		before := marshal(target)
		if err := Decode(value, target); err != nil {
			return err
		}
		if !bytes.Equal(before, marshal(target)) {
			switch field {
			case "completion":
				var old domain.Completion
				_ = json.Unmarshal(before, &old)
				t.Completion.Version = old.Version + 1
			case "execution_guidance":
				if t.ExecutionGuidance != nil {
					var old *domain.Guidance
					_ = json.Unmarshal(before, &old)
					t.ExecutionGuidance.Version = 1
					if old != nil {
						t.ExecutionGuidance.Version = old.Version + 1
					}
				}
			case "completion_check":
				if t.CompletionCheck != nil {
					var old *domain.CheckInstructions
					_ = json.Unmarshal(before, &old)
					t.CompletionCheck.Version = 1
					if old != nil {
						t.CompletionCheck.Version = old.Version + 1
					}
				}
			}
		}
	}
	return nil
}
func (s Service) Get(ctx context.Context, id string) (Bundle, error) {
	if !domain.ValidID(id) {
		return Bundle{}, domain.Err("invalid_input", "invalid task ID")
	}
	all, e := s.Repo.Snapshot(ctx)
	if e != nil {
		return Bundle{}, e
	}
	for _, b := range all {
		if b.Task.ID == id {
			return b, nil
		}
	}
	return Bundle{}, domain.Err("not_found", "task not found")
}
func (s Service) List(ctx context.Context, q domain.Query) (domain.Page, error) {
	all, e := s.Repo.Snapshot(ctx)
	if e != nil {
		return domain.Page{}, e
	}
	tasks := []domain.Task{}
	history := map[string][]domain.Event{}
	for _, b := range all {
		tasks = append(tasks, b.Task)
		history[b.Task.ID] = b.History
	}
	return domain.Select(tasks, history, q, s.now())
}

// MaxContextBytes bounds the JSON packet, independently of item counts.
const MaxContextBytes = 4 * 1024 * 1024

type Packet struct {
	Task                   domain.Task     `json:"task"`
	FollowUp               domain.FollowUp `json:"follow_up"`
	Unreconciled           []domain.Event  `json:"unreconciled_observations"`
	UnreconciledCount      int             `json:"unreconciled_count"`
	ObservationsOffset     int             `json:"observations_offset"`
	NextObservationsOffset *int            `json:"next_observations_offset,omitempty"`
	Recent                 []domain.Event  `json:"recent_history"`
	RecentTruncated        bool            `json:"recent_history_truncated"`
	Missing                []string        `json:"missing_context"`
	HistoryCount           int             `json:"history_count"`
}

func (s Service) Context(ctx context.Context, id string) (Packet, error) {
	return s.ContextPage(ctx, id, 0, 10, nil)
}

// ContextPage uses offsets into unresolved observations at one task revision.
// Continuation requires that revision so reconciliation cannot silently skip items.
func (s Service) ContextPage(ctx context.Context, id string, offset, limit int, expected *int64) (Packet, error) {
	if offset < 0 || limit < 1 || limit > 50 {
		return Packet{}, domain.Err("invalid_input", "context offset must be nonnegative and limit 1–50")
	}
	if offset > 0 && expected == nil {
		return Packet{}, domain.Err("invalid_input", "context continuation requires expected_revision")
	}
	b, e := s.Get(ctx, id)
	if e != nil {
		return Packet{}, e
	}
	if expected != nil && *expected != b.Task.Revision {
		return Packet{}, domain.Conflict(b.Task.Revision)
	}
	p := Packet{Task: b.Task, FollowUp: b.Task.FollowUp(b.History, s.now()), Unreconciled: []domain.Event{}, Recent: []domain.Event{}, Missing: []string{}, HistoryCount: len(b.History), ObservationsOffset: offset}
	if b.Task.Context.Objective == "" {
		p.Missing = append(p.Missing, "objective")
	}
	if len(b.Task.Completion.Criteria) == 0 {
		p.Missing = append(p.Missing, "completion.criteria")
	}
	if b.Task.ExecutionGuidance == nil {
		p.Missing = append(p.Missing, "execution_guidance")
	}
	if b.Task.CompletionCheck == nil {
		p.Missing = append(p.Missing, "completion_check")
	}
	seen := map[string]bool{}
	for _, ev := range b.History {
		for _, id := range ev.Acknowledges {
			seen[id] = true
		}
		for _, r := range []*domain.Report{ev.Report, ev.Check} {
			if r != nil {
				for _, id := range r.Reconciles {
					seen[id] = true
				}
			}
		}
	}
	unresolved := []domain.Event{}
	for _, ev := range b.History {
		if ev.Observation != nil && !seen[ev.EventID] {
			unresolved = append(unresolved, ev)
		}
	}
	p.UnreconciledCount = len(unresolved)
	if offset > len(unresolved) {
		return Packet{}, domain.Err("invalid_input", "context offset exceeds unresolved observations")
	}
	end := offset
	// Reserve space for the continuation metadata even if the packet fills exactly.
	withinBudget := func() bool { return len(marshal(p)) <= MaxContextBytes-256 }
	for end < len(unresolved) && len(p.Unreconciled) < limit {
		p.Unreconciled = append(p.Unreconciled, unresolved[end])
		if !withinBudget() {
			p.Unreconciled = p.Unreconciled[:len(p.Unreconciled)-1]
			break
		}
		end++
	}
	if end < len(unresolved) {
		p.NextObservationsOffset = &end
	}
	if end == offset && offset < len(unresolved) {
		return Packet{}, domain.Err("storage_error", "context cannot fit the next observation; inspect task history")
	}
	// Fill remaining space with at most ten recent events, newest first for selection,
	// then present them in chronological order. Never alter the durable events.
	for i := len(b.History) - 1; i >= 0 && len(p.Recent) < 10; i-- {
		p.Recent = append([]domain.Event{b.History[i]}, p.Recent...)
		if !withinBudget() {
			p.Recent = p.Recent[1:]
			break
		}
	}
	p.RecentTruncated = len(p.Recent) < len(b.History)
	if len(marshal(p)) > MaxContextBytes {
		return Packet{}, domain.Err("storage_error", "context exceeds response limit")
	}
	return p, nil
}
func (s Service) History(ctx context.Context, id string, offset, limit int) ([]domain.Event, error) {
	if offset < 0 || limit < 1 || limit > 200 {
		return nil, domain.Err("invalid_input", "invalid history range")
	}
	b, e := s.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	if offset > len(b.History) {
		offset = len(b.History)
	}
	end := offset + limit
	if end > len(b.History) {
		end = len(b.History)
	}
	return b.History[offset:end], nil
}
