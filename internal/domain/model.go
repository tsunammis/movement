// Package domain owns Movement task semantics independently of storage and transports.
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const SchemaVersion = 1
const (
	Candidate      = "candidate"
	Ready          = "ready"
	InProgress     = "in_progress"
	Blocked        = "blocked"
	SeemsCompleted = "seems_completed"
	Done           = "done"
	Dismissed      = "dismissed"
	Actionable     = "actionable"
	WaitingFor     = "waiting_for"
)

type Error struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Revision int64  `json:"revision,omitempty"`
}

func (e *Error) Error() string       { return e.Code + ": " + e.Message }
func Err(code, message string) error { return &Error{Code: code, Message: message} }
func Conflict(rev int64) error {
	return &Error{Code: "revision_conflict", Message: "reread task before retrying", Revision: rev}
}
func NewID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ValidID(s string) bool { return uuid.MatchString(s) }

type Context struct {
	Objective   string   `json:"objective"`
	Constraints []string `json:"constraints"`
	Notes       string   `json:"notes"`
}
type Criterion struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}
type Completion struct {
	Version  int64       `json:"version"`
	Criteria []Criterion `json:"criteria"`
}
type Reference struct {
	Name      string `json:"name,omitempty"`
	Reference string `json:"reference"`
}
type OutputKind struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
}
type Guidance struct {
	Version              int64        `json:"version"`
	Prompt               string       `json:"prompt"`
	Steps                []string     `json:"steps"`
	InputRefs            []Reference  `json:"input_refs"`
	RequiredCapabilities []string     `json:"required_capabilities"`
	ExpectedOutputs      []OutputKind `json:"expected_outputs"`
}
type CheckInstructions struct {
	Version      int64    `json:"version"`
	Prompt       string   `json:"prompt"`
	CriterionIDs []string `json:"criterion_ids"`
	SourceRefs   []string `json:"source_refs"`
}
type Person struct {
	DisplayName string `json:"display_name"`
	Reference   string `json:"reference,omitempty"`
}
type Delegation struct {
	Assignee        Person    `json:"assignee"`
	DelegatedAt     time.Time `json:"delegated_at"`
	ExpectedOutcome string    `json:"expected_outcome"`
}
type Source struct {
	Provider   string    `json:"provider"`
	Reference  string    `json:"reference"`
	CapturedAt time.Time `json:"captured_at"`
	Summary    string    `json:"summary"`
}
type Ingestion struct {
	Producer    string `json:"producer"`
	ExternalKey string `json:"external_key"`
}
type Claim struct {
	AttemptID string    `json:"attempt_id"`
	Actor     string    `json:"actor"`
	TokenHash string    `json:"token_hash"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Task struct {
	SchemaVersion     int                `json:"schema_version"`
	ID                string             `json:"id"`
	Revision          int64              `json:"revision"`
	ContextVersion    int64              `json:"context_version"`
	LastEventID       string             `json:"last_event_id"`
	Title             string             `json:"title"`
	Description       string             `json:"description"`
	Status            string             `json:"status"`
	Flow              string             `json:"flow"`
	Priority          string             `json:"priority"`
	Tags              []string           `json:"tags"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	CreatedBy         string             `json:"created_by"`
	ScheduledFor      *time.Time         `json:"scheduled_for"`
	DueAt             *time.Time         `json:"due_at"`
	FollowUpAt        *time.Time         `json:"follow_up_at"`
	Context           Context            `json:"context"`
	Completion        Completion         `json:"completion"`
	ExecutionGuidance *Guidance          `json:"execution_guidance,omitempty"`
	CompletionCheck   *CheckInstructions `json:"completion_check,omitempty"`
	Delegation        *Delegation        `json:"delegation,omitempty"`
	Sources           []Source           `json:"sources"`
	Ingestion         *Ingestion         `json:"ingestion,omitempty"`
	Claim             *Claim             `json:"claim,omitempty"`
}

func (t Task) Terminal() bool                 { return t.Status == Done || t.Status == Dismissed }
func (t Task) ActiveClaim(now time.Time) bool { return t.Claim != nil && t.Claim.ExpiresAt.After(now) }
func (t Task) Executable(now time.Time) bool {
	return t.Flow == Actionable && (t.Status == Ready || (t.Status == InProgress && t.Claim != nil && !t.ActiveClaim(now))) && !t.ActiveClaim(now) && strings.TrimSpace(t.Context.Objective) != "" && len(t.Completion.Criteria) > 0
}
func (t *Task) Normalize() {
	if t.Flow == "" {
		t.Flow = Actionable
	}
	if t.Status == "" {
		t.Status = Candidate
	}
	if t.Priority == "" {
		t.Priority = "normal"
	}
	if t.Completion.Version == 0 {
		t.Completion.Version = 1
	}
	if t.Completion.Criteria == nil {
		t.Completion.Criteria = []Criterion{}
	}
	if t.Tags == nil {
		t.Tags = []string{}
	}
	if t.Sources == nil {
		t.Sources = []Source{}
	}
	if t.Context.Constraints == nil {
		t.Context.Constraints = []string{}
	}
	for _, p := range []**time.Time{&t.ScheduledFor, &t.DueAt, &t.FollowUpAt} {
		if *p != nil {
			utc := (*p).UTC()
			*p = &utc
		}
	}
	t.CreatedAt = t.CreatedAt.UTC()
	t.UpdatedAt = t.UpdatedAt.UTC()
	if t.Delegation != nil {
		delegation := *t.Delegation
		delegation.DelegatedAt = delegation.DelegatedAt.UTC()
		t.Delegation = &delegation
	}
	if t.Claim != nil {
		claim := *t.Claim
		claim.ExpiresAt = claim.ExpiresAt.UTC()
		t.Claim = &claim
	}
	t.Sources = append([]Source{}, t.Sources...)
	for i := range t.Sources {
		t.Sources[i].CapturedAt = t.Sources[i].CapturedAt.UTC()
	}
}
func (t Task) Validate() error {
	if t.SchemaVersion != SchemaVersion {
		return Err("unsupported_schema", "unsupported task schema")
	}
	if !ValidID(t.ID) || t.Revision < 1 || t.ContextVersion < 1 || !ValidID(t.LastEventID) {
		return Err("invalid_input", "invalid task identity or revision")
	}
	if strings.TrimSpace(t.Title) == "" || len(t.Title) > 500 {
		return Err("invalid_input", "title must contain 1–500 bytes")
	}
	if len(t.Description) > 65536 || len(t.Context.Notes) > 65536 || len(t.Context.Objective) > 16384 {
		return Err("invalid_input", "task context exceeds limit")
	}
	if !contains([]string{Candidate, Ready, InProgress, Blocked, SeemsCompleted, Done, Dismissed}, t.Status) || !contains([]string{Actionable, WaitingFor}, t.Flow) || !contains([]string{"low", "normal", "high"}, t.Priority) {
		return Err("invalid_input", "invalid status, flow or priority")
	}
	if t.CreatedAt.IsZero() || t.UpdatedAt.IsZero() || strings.TrimSpace(t.CreatedBy) == "" {
		return Err("invalid_input", "creation metadata required")
	}
	for _, source := range t.Sources {
		if strings.TrimSpace(source.Provider) == "" || strings.TrimSpace(source.Reference) == "" || source.CapturedAt.IsZero() {
			return Err("invalid_input", "sources require provider, reference and capture time")
		}
		if len(source.Summary) > 65536 {
			return Err("invalid_input", "source summary exceeds 65536 bytes")
		}
	}
	if t.Completion.Version < 1 || len(t.Completion.Criteria) > 100 {
		return Err("invalid_input", "invalid completion version or criterion count")
	}
	ids := map[string]bool{}
	for _, c := range t.Completion.Criteria {
		if c.ID == "" || len(c.ID) > 100 || strings.TrimSpace(c.Description) == "" || ids[c.ID] {
			return Err("invalid_input", "criteria require unique IDs and descriptions")
		}
		ids[c.ID] = true
	}
	if t.CompletionCheck != nil {
		c := t.CompletionCheck
		if c.Version < 1 || strings.TrimSpace(c.Prompt) == "" {
			return Err("invalid_input", "invalid check instructions")
		}
		seen := map[string]bool{}
		for _, id := range c.CriterionIDs {
			if !ids[id] || seen[id] {
				return Err("invalid_input", "unknown/duplicate check criterion")
			}
			seen[id] = true
		}
	}
	if t.ExecutionGuidance != nil && (t.ExecutionGuidance.Version < 1 || strings.TrimSpace(t.ExecutionGuidance.Prompt) == "") {
		return Err("invalid_input", "invalid execution guidance")
	}
	if t.Flow == WaitingFor && (t.Delegation == nil || strings.TrimSpace(t.Delegation.Assignee.DisplayName) == "" || t.Delegation.DelegatedAt.IsZero() || strings.TrimSpace(t.Delegation.ExpectedOutcome) == "") {
		return Err("invalid_input", "waiting_for requires delegate, time and expected outcome")
	}
	if t.Claim != nil && (t.Flow != Actionable || t.Status != InProgress || !ValidID(t.Claim.AttemptID) || t.Claim.TokenHash == "" || t.Claim.ExpiresAt.IsZero()) {
		return Err("invalid_input", "claim requires actionable in_progress task")
	}
	if t.Ingestion != nil {
		var key []string
		if t.Ingestion.Producer == "" || json.Unmarshal([]byte(t.Ingestion.ExternalKey), &key) != nil || len(key) != 4 {
			return Err("invalid_input", "ingestion key must be [provider,source,action,occurrence]")
		}
		for _, s := range key {
			if strings.TrimSpace(s) == "" {
				return Err("invalid_input", "empty ingestion identity component")
			}
		}
		canonical, _ := json.Marshal(key)
		if string(canonical) != t.Ingestion.ExternalKey {
			return Err("invalid_input", "ingestion key must use canonical compact JSON")
		}
	}
	return nil
}
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
func (t *Task) Transition(action string) error {
	var next string
	switch action {
	case "accept":
		if t.Status == Candidate {
			next = Ready
		}
	case "dismiss":
		if !t.Terminal() {
			next = Dismissed
		}
	case "complete":
		if !t.Terminal() && t.Status != Candidate {
			next = Done
		}
	case "reopen":
		if t.Terminal() {
			next = Ready
		}
	case "resume":
		if t.Status == Blocked {
			next = Ready
		}
	case "needs_more_work":
		if t.Status == SeemsCompleted {
			next = Ready
		}
	default:
		return Err("invalid_input", "unknown transition")
	}
	if next == "" {
		return Err("invalid_transition", fmt.Sprintf("cannot %s from %s", action, t.Status))
	}
	t.Status = next
	if next != InProgress {
		t.Claim = nil
	}
	return nil
}

// SemanticContext deliberately excludes revisions, scheduling and coordination metadata.
func (t Task) SemanticContext() []byte {
	b, _ := json.Marshal(struct {
		Description, Flow string
		Context           Context
		Completion        Completion
		Guidance          *Guidance
		Check             *CheckInstructions
		Delegation        *Delegation
		Sources           []Source
	}{t.Description, t.Flow, t.Context, t.Completion, t.ExecutionGuidance, t.CompletionCheck, t.Delegation, t.Sources})
	return b
}

type Evidence struct {
	Reference   string    `json:"reference"`
	ObservedAt  time.Time `json:"observed_at"`
	Description string    `json:"description"`
	Excerpt     string    `json:"excerpt,omitempty"`
}
type Output struct {
	Kind        string    `json:"kind"`
	Provider    string    `json:"provider"`
	Reference   string    `json:"reference"`
	ObservedAt  time.Time `json:"observed_at"`
	Description string    `json:"description"`
}
type CriterionResult struct {
	CriterionID string     `json:"criterion_id"`
	Result      string     `json:"result"`
	Evidence    []Evidence `json:"evidence"`
}
type Report struct {
	AttemptID         string            `json:"attempt_id,omitempty"`
	ContextVersion    int64             `json:"context_version"`
	CompletionVersion int64             `json:"completion_version"`
	GuidanceVersion   int64             `json:"guidance_version,omitempty"`
	CheckVersion      int64             `json:"check_version,omitempty"`
	TaskRevisionUsed  int64             `json:"task_revision_used"`
	ObservedAt        time.Time         `json:"observed_at"`
	Outcome           string            `json:"outcome"`
	Summary           string            `json:"summary"`
	CriterionResults  []CriterionResult `json:"criterion_results"`
	Evidence          []Evidence        `json:"evidence"`
	Outputs           []Output          `json:"outputs"`
	NextSteps         string            `json:"next_steps,omitempty"`
	Reconciles        []string          `json:"reconciles,omitempty"`
}

func (t Task) Aggregate(results []CriterionResult, now time.Time) (string, error) {
	known := map[string]bool{}
	for _, c := range t.Completion.Criteria {
		known[c.ID] = true
	}
	seen := map[string]string{}
	for _, r := range results {
		if !known[r.CriterionID] || seen[r.CriterionID] != "" || !contains([]string{"met", "not_met", "unknown"}, r.Result) {
			return "", Err("invalid_input", "unknown/duplicate criterion or invalid result")
		}
		seen[r.CriterionID] = r.Result
		if r.Result == "met" && len(r.Evidence) == 0 {
			return "", Err("invalid_input", "met requires evidence")
		}
		if err := ValidateEvidence(r.Evidence, now); err != nil {
			return "", err
		}
	}
	all := len(known) > 0
	for id := range known {
		if seen[id] == "not_met" {
			return "not_met", nil
		}
		if seen[id] != "met" {
			all = false
		}
	}
	if all {
		return "met", nil
	}
	return "inconclusive", nil
}
func ValidateEvidence(evidence []Evidence, now time.Time) error {
	for _, e := range evidence {
		if e.Reference == "" || e.ObservedAt.IsZero() || e.ObservedAt.After(now) {
			return Err("invalid_input", "evidence requires reference and non-future observation time")
		}
	}
	return nil
}

// ValidateReportArtifacts applies to accepted reports and retained late evidence.
// It validates structure without applying current criteria to historical context.
func ValidateReportArtifacts(r Report, now time.Time) error {
	if err := ValidateEvidence(r.Evidence, now); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, c := range r.CriterionResults {
		if c.CriterionID == "" || seen[c.CriterionID] || !contains([]string{"met", "not_met", "unknown"}, c.Result) {
			return Err("invalid_input", "invalid or duplicate historical criterion result")
		}
		seen[c.CriterionID] = true
		if c.Result == "met" && len(c.Evidence) == 0 {
			return Err("invalid_input", "met requires evidence")
		}
		if err := ValidateEvidence(c.Evidence, now); err != nil {
			return err
		}
	}
	for _, o := range r.Outputs {
		if o.Kind == "" || o.Provider == "" || o.Reference == "" || o.ObservedAt.IsZero() || o.ObservedAt.After(now) {
			return Err("invalid_input", "invalid output reference")
		}
	}
	return nil
}
func (t Task) ValidateReport(r Report, check bool, now time.Time) error {
	if t.Terminal() || t.Status == Candidate {
		return Err("invalid_transition", "task is not open for reports")
	}
	if r.ContextVersion != t.ContextVersion || r.CompletionVersion != t.Completion.Version {
		return Err("context_conflict", "reevaluate current task context")
	}
	if r.TaskRevisionUsed < 1 || r.TaskRevisionUsed > t.Revision || r.ObservedAt.IsZero() || r.ObservedAt.After(now) || strings.TrimSpace(r.Summary) == "" {
		return Err("invalid_input", "report requires evaluated revision, summary and non-future observation time")
	}
	if check {
		if t.CompletionCheck == nil || r.CheckVersion != t.CompletionCheck.Version {
			return Err("context_conflict", "check instructions changed")
		}
	} else {
		if t.ExecutionGuidance != nil && r.GuidanceVersion != t.ExecutionGuidance.Version {
			return Err("context_conflict", "guidance changed")
		}
		if t.Claim == nil || r.AttemptID != t.Claim.AttemptID {
			return Err("claim_conflict", "report must reference current attempt")
		}
	}
	agg, err := t.Aggregate(r.CriterionResults, now)
	if err != nil {
		return err
	}
	if err = ValidateReportArtifacts(r, now); err != nil {
		return err
	}
	if check {
		if r.Outcome != agg {
			return Err("invalid_input", "reported outcome differs from criterion aggregate")
		}
	} else if !contains([]string{"progress", "blocked", "completion_proposed"}, r.Outcome) {
		return Err("invalid_input", "unknown work outcome")
	} else if r.Outcome == "completion_proposed" && agg != "met" {
		return Err("invalid_input", "completion requires all criteria met")
	}
	return nil
}

type Event struct {
	SchemaVersion  int                        `json:"schema_version"`
	TaskID         string                     `json:"task_id"`
	EventID        string                     `json:"event_id"`
	Sequence       int64                      `json:"sequence"`
	Revision       int64                      `json:"revision"`
	ContextVersion int64                      `json:"context_version"`
	Type           string                     `json:"type"`
	Actor          string                     `json:"actor"`
	RecordedAt     time.Time                  `json:"recorded_at"`
	OperationID    string                     `json:"operation_id"`
	InputDigest    string                     `json:"input_digest"`
	Summary        string                     `json:"summary"`
	Changes        map[string]json.RawMessage `json:"changes,omitempty"`
	Report         *Report                    `json:"report,omitempty"`
	Check          *Report                    `json:"check,omitempty"`
	Observation    *Report                    `json:"observation,omitempty"`
	AttemptID      string                     `json:"attempt_id,omitempty"`
	Acknowledges   []string                   `json:"acknowledges,omitempty"`
}

func EventTypes() []string {
	return []string{"task_created", "task_updated", "task_claimed", "claim_renewed", "claim_released", "work_reported", "completion_checked", "observation_recorded", "observation_acknowledged"}
}
func ValidEventType(s string) bool { return contains(EventTypes(), s) }

// NormalizeTimestamps preserves instants and prose while canonicalizing stored times.
func (r *Report) NormalizeTimestamps() {
	r.ObservedAt = r.ObservedAt.UTC()
	for i := range r.Evidence {
		r.Evidence[i].ObservedAt = r.Evidence[i].ObservedAt.UTC()
	}
	for i := range r.Outputs {
		r.Outputs[i].ObservedAt = r.Outputs[i].ObservedAt.UTC()
	}
	for i := range r.CriterionResults {
		for j := range r.CriterionResults[i].Evidence {
			e := &r.CriterionResults[i].Evidence[j]
			e.ObservedAt = e.ObservedAt.UTC()
		}
	}
}
