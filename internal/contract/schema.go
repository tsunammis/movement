// Package contract generates structural JSON contracts from the shared Go models.
// Cross-field, temporal and transactional invariants remain in domain/app validation.
package contract

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
)

type Object = map[string]any

const Draft = "https://json-schema.org/draft/2020-12/schema"
const UUID = "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"

var statuses = []string{"candidate", "ready", "in_progress", "blocked", "seems_completed", "done", "dismissed"}

// Schema emits exact known fields. Stored/output models require non-omitempty
// fields; input models allow omissions that the shared service normalizes.
func Schema(t reflect.Type, output bool) Object {
	if t == reflect.TypeFor[json.RawMessage]() {
		return Object{}
	}
	if t == reflect.TypeFor[time.Time]() {
		return Object{"type": "string", "format": "date-time"}
	}
	if t.Kind() == reflect.Pointer {
		return Object{"anyOf": []any{Schema(t.Elem(), output), Object{"type": "null"}}}
	}
	switch t.Kind() {
	case reflect.Struct:
		props := Object{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")
			name := tag[0]
			if name == "" || name == "-" {
				continue
			}
			props[name] = Schema(f.Type, output)
			if output && !strings.Contains(f.Tag.Get("json"), "omitempty") {
				required = append(required, name)
			}
		}
		result := Object{"type": "object", "properties": props, "additionalProperties": false}
		if len(required) > 0 {
			result["required"] = required
		}
		refine(t, props, result, output)
		return result
	case reflect.Slice, reflect.Array:
		s := Object{"type": "array", "items": Schema(t.Elem(), output)}
		if t.Kind() == reflect.Slice {
			return Object{"anyOf": []any{s, Object{"type": "null"}}}
		}
		return s
	case reflect.Map:
		return Object{"type": "object", "additionalProperties": Schema(t.Elem(), output)}
	case reflect.Bool:
		return Object{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return Object{"type": "integer"}
	default:
		return Object{"type": "string"}
	}
}
func refine(t reflect.Type, p, result Object, output bool) {
	set := func(name string, extra Object) {
		if v, ok := p[name].(map[string]any); ok {
			for k, x := range extra {
				v[k] = x
			}
		}
	}
	switch t {
	case reflect.TypeFor[domain.Task](), reflect.TypeFor[app.Create]():
		set("title", Object{"minLength": 1, "maxLength": 500})
		set("status", Object{"enum": statuses})
		set("flow", Object{"enum": []string{"actionable", "waiting_for"}})
		set("priority", Object{"enum": []string{"low", "normal", "high"}})
		if t == reflect.TypeFor[app.Create]() {
			result["required"] = []string{"title"}
			set("status", Object{"enum": []string{"candidate", "ready"}})
		} else {
			set("schema_version", Object{"const": 1})
			set("id", Object{"pattern": UUID})
			set("last_event_id", Object{"pattern": UUID})
			set("revision", Object{"minimum": 1})
			set("context_version", Object{"minimum": 1})
		}
	case reflect.TypeFor[domain.Event]():
		set("schema_version", Object{"const": 1})
		for _, key := range []string{"task_id", "event_id"} {
			set(key, Object{"pattern": UUID})
		}
		for _, key := range []string{"sequence", "revision", "context_version"} {
			set(key, Object{"minimum": 1})
		}
		set("input_digest", Object{"pattern": "^[0-9a-f]{64}$"})
		set("type", Object{"enum": domain.EventTypes()})
	case reflect.TypeFor[domain.Source]():
		if !output {
			result["required"] = []string{"provider", "reference", "captured_at"}
		}
		set("provider", Object{"minLength": 1})
		set("reference", Object{"minLength": 1})
		set("summary", Object{"maxLength": 65536})
	case reflect.TypeFor[domain.Report]():
		set("outcome", Object{"enum": []string{"progress", "blocked", "completion_proposed", "met", "not_met", "inconclusive"}, "description": "Work reports use progress, blocked or completion_proposed. Check reports use met, not_met or inconclusive. Late observations retain their original report outcome."})
	case reflect.TypeFor[domain.CriterionResult]():
		set("result", Object{"enum": []string{"met", "not_met", "unknown"}})
		set("criterion_id", Object{"minLength": 1})
	case reflect.TypeFor[domain.Completion](), reflect.TypeFor[domain.Guidance](), reflect.TypeFor[domain.CheckInstructions]():
		if output {
			set("version", Object{"minimum": 1})
		}
	case reflect.TypeFor[domain.Query]():
		set("view", Object{"enum": []string{"planning", "execution"}})
		set("flow", Object{"enum": []string{"actionable", "waiting_for", "all"}})
		set("time_field", Object{"enum": []string{"delivery", "follow_up"}})
		set("limit", Object{"minimum": 0, "maximum": 200})
	case reflect.TypeFor[app.Update]():
		set("action", Object{"enum": []string{"accept", "dismiss", "complete", "reopen", "resume", "needs_more_work", ""}})
		task := Schema(reflect.TypeFor[domain.Task](), false)["properties"].(map[string]any)
		fields := Object{}
		for _, name := range []string{"title", "description", "flow", "priority", "tags", "scheduled_for", "due_at", "follow_up_at", "context", "completion", "execution_guidance", "completion_check", "delegation", "sources"} {
			fields[name] = task[name]
		}
		p["fields"] = Object{"type": "object", "properties": fields, "additionalProperties": false}
	case reflect.TypeFor[app.ReportInput]():
		result["required"] = []string{"report"}
		p["next_follow_up_at"] = Schema(reflect.TypeFor[*time.Time](), false)
	case reflect.TypeFor[app.Receipt]():
		set("input_digest", Object{"pattern": "^[0-9a-f]{64}$"})
		set("task_id", Object{"pattern": UUID})
		set("event_id", Object{"pattern": UUID})
		set("revision", Object{"minimum": 1})
	}
}
func Document(title string, t reflect.Type) Object {
	s := Schema(t, true)
	s["$schema"] = Draft
	s["title"] = title
	return s
}
