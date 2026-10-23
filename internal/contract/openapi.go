package contract

import (
	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
	"reflect"
)

func ref(name string) Object    { return Object{"$ref": "#/components/schemas/" + name} }
func content(schema any) Object { return Object{"application/json": Object{"schema": schema}} }
func parameter(name, where string, schema any, required bool) Object {
	return Object{"name": name, "in": where, "required": required, "schema": schema}
}

func OpenAPI() Object {
	schemas := Object{
		"Task": Schema(reflect.TypeFor[domain.Task](), true), "HistoryEvent": Schema(reflect.TypeFor[domain.Event](), true), "Receipt": Schema(reflect.TypeFor[app.Receipt](), true), "ContextPacket": Schema(reflect.TypeFor[app.Packet](), true), "SelectionPage": Schema(reflect.TypeFor[domain.Page](), true),
		"Error":   Object{"type": "object", "required": []string{"error"}, "properties": Object{"error": Schema(reflect.TypeFor[domain.Error](), true)}, "additionalProperties": false},
		"Session": Object{"type": "object", "required": []string{"csrf_token", "expires_at"}, "properties": Object{"csrf_token": Object{"type": "string"}, "expires_at": Object{"type": "string", "format": "date-time"}}},
	}
	paths := Object{}
	bearer := Object{"bearerAuth": []string{}}
	browser := Object{"browserSession": []string{}}
	responses := func(schema any) Object {
		out := Object{"200": Object{"description": "Success; mutations return the original operation receipt on identical retries", "content": content(schema)}}
		for code, description := range map[string]string{"400": "Invalid input", "401": "Authentication required", "403": "Host, Origin or CSRF rejected", "404": "Resource not found", "405": "Unsupported method", "409": "Revision, context, assessment, claim, operation or lifecycle conflict", "422": "Unsupported schema", "500": "Storage or internal error", "503": "Store busy"} {
			out[code] = Object{"description": description, "content": content(ref("Error"))}
		}
		return out
	}
	add := func(path, method, id string, parameters []any, body, output any) {
		op := Object{"operationId": id, "responses": responses(output)}
		if len(parameters) > 0 {
			op["parameters"] = parameters
		}
		if body != nil {
			op["requestBody"] = Object{"required": true, "content": content(body)}
		}
		if _, ok := paths[path]; !ok {
			paths[path] = Object{}
		}
		paths[path].(map[string]any)[method] = op
	}
	taskID := parameter("id", "path", Object{"type": "string", "pattern": UUID}, true)
	reads := []struct {
		path, id string
		schema   any
	}{{"/tasks/{id}", "task_get", ref("Task")}, {"/tasks/{id}/context", "task_context", ref("ContextPacket")}, {"/tasks/{id}/history", "task_history", Object{"type": "array", "items": ref("HistoryEvent")}}}
	for _, r := range reads {
		params := []any{taskID}
		if r.id != "task_get" {
			max, def := 200, 50
			if r.id == "task_context" {
				max, def = 50, 10
			}
			params = append(params, parameter("offset", "query", Object{"type": "integer", "minimum": 0, "default": 0}, false), parameter("limit", "query", Object{"type": "integer", "minimum": 1, "maximum": max, "default": def}, false))
			if r.id == "task_context" {
				params = append(params, parameter("expected_revision", "query", Object{"type": "integer", "minimum": 1, "description": "Required when offset > 0; use the first packet task revision"}, false))
			}
		}
		add(r.path, "get", r.id, params, nil, r.schema)
	}
	q := Schema(reflect.TypeFor[domain.Query](), false)["properties"].(map[string]any)
	params := []any{}
	// Stable order for reviewable generated artifacts.
	for _, name := range []string{"from", "to", "as_of", "view", "flow", "statuses", "assignee", "time_field", "include_overdue", "include_carryover", "include_unscheduled", "undated_only", "pending_only", "limit", "cursor"} {
		s := q[name]
		if name == "from" || name == "to" {
			s = Object{"type": "string", "description": "RFC3339 instant or paired date-only boundaries in repository timezone; to is exclusive"}
		}
		p := parameter(name, "query", s, false)
		if name == "statuses" {
			p["style"] = "form"
			p["explode"] = true
		}
		params = append(params, p)
	}
	add("/tasks", "get", "task_list", params, nil, ref("SelectionPage"))
	mutations := []struct {
		path, method, id string
		data             any
	}{{"/tasks", "post", "task_create", app.Create{}}, {"/tasks/{id}", "patch", "task_update", app.Update{}}, {"/tasks/{id}/claim", "post", "task_claim", app.Lease{}}, {"/tasks/{id}/claim/renew", "post", "task_renew", app.Lease{}}, {"/tasks/{id}/claim/release", "post", "task_release", struct{}{}}, {"/tasks/{id}/reports", "post", "task_report", app.ReportInput{}}, {"/tasks/{id}/checks", "post", "task_check_report", app.ReportInput{}}, {"/tasks/{id}/observations", "post", "task_observe", app.Observation{}}}
	for _, m := range mutations {
		required := []string{"operation_id", "actor", "data"}
		params := []any{parameter("Origin", "header", Object{"type": "string", "description": "Exact local origin, required with browser cookie mutations"}, false), parameter("X-CSRF-Token", "header", Object{"type": "string", "description": "Required with browser cookie mutations"}, false)}
		if m.id != "task_create" {
			required = append(required, "expected_revision")
			params = append(params, taskID)
		}
		envelope := Object{"type": "object", "additionalProperties": false, "required": required, "properties": Object{"operation_id": Object{"type": "string", "minLength": 1, "maxLength": 200}, "actor": Object{"type": "string", "minLength": 1, "maxLength": 200}, "expected_revision": Object{"type": "integer", "minimum": 1}, "claim_token": Object{"type": "string"}, "override": Object{"type": "boolean", "default": false}, "data": Schema(reflect.TypeOf(m.data), false)}}
		add(m.path, m.method, m.id, params, envelope, ref("Receipt"))
	}
	add("/config", "get", "get_config", nil, nil, Object{"type": "object", "required": []string{"timezone"}, "properties": Object{"timezone": Object{"type": "string"}}})
	add("/session", "post", "create_session", nil, nil, ref("Session"))
	paths["/session"].(map[string]any)["post"].(map[string]any)["security"] = []any{bearer}
	add("/session", "get", "get_session", nil, nil, ref("Session"))
	paths["/session"].(map[string]any)["get"].(map[string]any)["security"] = []any{browser}
	add("/session", "delete", "delete_session", []any{parameter("Origin", "header", Object{"type": "string"}, true), parameter("X-CSRF-Token", "header", Object{"type": "string"}, true)}, nil, Object{"type": "object", "required": []string{"logged_out"}, "properties": Object{"logged_out": Object{"const": true}}})
	paths["/session"].(map[string]any)["delete"].(map[string]any)["security"] = []any{browser}
	return Object{"openapi": "3.1.0", "info": Object{"title": "Movement local API", "version": "0.1.0", "description": "Loopback only. Bodies are limited to 1 MiB. Bearer or HttpOnly SameSite=Strict session authentication. Cookie mutations also require exact Origin and CSRF token. Context packets are bounded at 4 MiB with revision-checked unresolved-observation pagination. Cross-field lifecycle, time, claim and transaction invariants are enforced by the shared application service."}, "servers": []any{Object{"url": "http://127.0.0.1:8484/api/v1"}}, "security": []any{bearer, browser}, "paths": paths, "components": Object{"schemas": schemas, "securitySchemes": Object{"bearerAuth": Object{"type": "http", "scheme": "bearer"}, "browserSession": Object{"type": "apiKey", "in": "cookie", "name": "movement_session"}}}}
}
