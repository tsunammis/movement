# Local HTTP API

Start an initialized repository with:

```sh
movement --data-dir /absolute/path/to/tasks serve --listen 127.0.0.1:8484
```

CLI and MCP remain independent processes; they do not need this server. `serve` binds only a literal loopback IP and port. Port 0 selects an ephemeral port. The startup URL is printed to stderr; credentials are never included in URLs or startup logs. SIGTERM/SIGINT initiates graceful shutdown.

Retrieve the local credential deliberately with `movement --data-dir /absolute/path/to/tasks auth token`. API clients send it in `Authorization: Bearer <token>`. The credential is stored privately in `runtime/api-token`; do not put it in task records or logs.

## Routes and payloads

All routes are under `/api/v1` and require authentication except browser session creation, which itself requires the bearer credential. Mutation bodies require `Content-Type: application/json` and are limited to 1 MiB. Unknown/duplicate JSON fields are rejected.

| Route | Method | Shared use case |
| --- | --- | --- |
| `/tasks` | GET / POST | List / create |
| `/tasks/{id}` | GET / PATCH | Current task / update |
| `/tasks/{id}/context` | GET | Agent context |
| `/tasks/{id}/history` | GET | History with offset and limit |
| `/tasks/{id}/claim` | POST | Claim |
| `/tasks/{id}/claim/renew` | POST | Renew |
| `/tasks/{id}/claim/release` | POST | Release |
| `/tasks/{id}/reports` | POST | Work report |
| `/tasks/{id}/checks` | POST | Check report |
| `/tasks/{id}/observations` | POST | Late evidence / acknowledgement |
| `/config` | GET | Repository timezone |

Mutations use `{operation_id, actor, expected_revision?, claim_token?, override?, data}`. The route supplies task identity and operation kind. `data` follows the [CLI payload contracts](cli.md). Successful mutations return the original receipt as JSON with HTTP 200, including idempotent retries. Reads return the task, context, history array or selection page directly.

List query parameters use the names in the spec (`from`, `to`, `as_of`, `view`, `flow`, etc.). Dates are RFC3339 instants, or a pair of date-only from/to values converted by the server using the repository timezone. Repeat `statuses` for multiple values. Other duplicate or unknown parameters fail validation. Preserve returned `as_of` across pages. History accepts offset (default 0) and limit (default 50, maximum 200). Context accepts offset (default 0), limit (default 10, maximum 50), and expected_revision (required for continuation); its response identifies remaining unresolved observations.

Failures return `{error: {code, message, revision?}}`. Invalid inputs map to 400, missing credentials to 401, origin/CSRF violations to 403, missing tasks to 404, unsupported methods to 405, revision/claim/operation/assessment conflicts to 409, unsupported schemas to 422, lock contention to 503, and unexpected failures to 500. Reread and reconcile conflicts; do not blindly retry stale writes.

## Browser session contract

The browser exchanges the bearer credential through `POST /session`. The response sets a random HttpOnly, SameSite=Strict cookie scoped to `/api/v1` and returns `csrf_token` and `expires_at`. Keep the entered bearer token only long enough to exchange it; do not store it in localStorage or a URL. Sessions expire after eight hours, are held only in server memory, and disappear on restart. At most 256 active sessions are allowed. Plain loopback HTTP cannot use a Secure cookie; remote serving is unsupported.

`GET /session` restores the CSRF token for an existing session. Cookie-authenticated mutations, including `DELETE /session` logout, require the exact approved `Origin` and `X-CSRF-Token`. Bearer requests do not require CSRF tokens, but supplied hostile origins are still rejected. Logout invalidates server state and clears the cookie.

Host validation accepts only the bound address. Production Origin validation accepts only its HTTP origin. Vite development may opt into one exact loopback origin using `--dev-origin http://localhost:5173`; its Host is also accepted for proxy requests. The proxy must preserve authentication, cookies, Origin and CSRF headers. No wildcard CORS headers are emitted. Production does not enable this exception by default.

## Verification and limits

HTTP handler tests cover bearer and cookie access, Host/Origin rejection, CSRF, expiry/logout, strict input, retry receipts, query validation and revision conflicts against a real temporary repository. The root race suite and vet pass. `python3 scripts/http-smoke.py /absolute/path/to/movement` tests a real loopback listener, HTTP creation, independent CLI readback and graceful shutdown.

The embedded UI and Vite proxy browser workflow are implemented and tested at the scope in [UI documentation](ui.md). OpenAPI, exhaustive mutation parity and credential rotation/restore handling remain release requirements.
