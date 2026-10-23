// Package httpapi serves the authenticated loopback API.
package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
)

const cookieName = "movement_session"

type session struct {
	CSRF    string
	Expires time.Time
}
type Options struct {
	Host, Token, DevOrigin, Timezone string
	UI                               http.Handler
}
type Handler struct {
	svc      app.Service
	opts     Options
	mu       sync.Mutex
	sessions map[string]session
}

func New(svc app.Service, o Options) (*Handler, error) {
	host, _, e := net.SplitHostPort(o.Host)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, fmt.Errorf("HTTP host must be a loopback IP and port")
	}
	if len(o.Token) < 32 {
		return nil, fmt.Errorf("API token missing or too short")
	}
	if o.DevOrigin != "" {
		u, e := url.Parse(o.DevOrigin)
		if e != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Port() == "" || (u.Hostname() != "localhost" && (net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback())) {
			return nil, fmt.Errorf("development origin must be an exact loopback HTTP origin with port")
		}
	}
	return &Handler{svc: svc, opts: o, sessions: map[string]session{}}, nil
}
func secret() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error) {
	var de *domain.Error
	if !errors.As(err, &de) {
		de = &domain.Error{Code: "internal_error", Message: "Movement could not complete the operation"}
	}
	status := 500
	switch de.Code {
	case "invalid_input":
		status = 400
	case "unauthenticated":
		status = 401
	case "forbidden":
		status = 403
	case "not_found":
		status = 404
	case "method_not_allowed":
		status = 405
	case "revision_conflict", "context_conflict", "assessment_conflict", "operation_conflict", "invalid_transition", "claim_conflict":
		status = 409
	case "store_busy":
		status = 503
	case "unsupported_schema":
		status = 422
	}
	write(w, status, map[string]any{"error": de})
}
func body(w http.ResponseWriter, r *http.Request, v any) error {
	typ, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || typ != "application/json" {
		return domain.Err("invalid_input", "Content-Type application/json required")
	}
	b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024*1024))
	if e != nil {
		return domain.Err("invalid_input", "cannot read request body (maximum 1 MiB)")
	}
	return app.Decode(b, v)
}
func (h *Handler) originOK(origin string) bool {
	return origin == "http://"+h.opts.Host || (h.opts.DevOrigin != "" && origin == h.opts.DevOrigin)
}
func (h *Handler) bearer(r *http.Request) bool {
	return equal(r.Header.Get("Authorization"), "Bearer "+h.opts.Token)
}
func (h *Handler) lookup(r *http.Request) (session, bool) {
	c, e := r.Cookie(cookieName)
	if e != nil {
		return session{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[c.Value]
	if ok && !time.Now().Before(s.Expires) {
		delete(h.sessions, c.Value)
		ok = false
	}
	return s, ok
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	hostOK := r.Host == h.opts.Host
	if h.opts.DevOrigin != "" {
		u, _ := url.Parse(h.opts.DevOrigin)
		hostOK = hostOK || r.Host == u.Host
	}
	if !hostOK {
		fail(w, domain.Err("forbidden", "unexpected Host"))
		return
	}
	origin := r.Header.Get("Origin")
	if origin != "" && !h.originOK(origin) {
		fail(w, domain.Err("forbidden", "unexpected Origin"))
		return
	}
	// No cross-origin CORS access. Vite uses a same-origin proxy.
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		if h.opts.UI != nil {
			h.opts.UI.ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	if r.URL.Path == "/api/v1/session" {
		h.session(w, r)
		return
	}
	s, authenticated := h.lookup(r)
	if r.Header.Get("Authorization") != "" {
		authenticated = h.bearer(r)
		s = session{}
	}
	if !authenticated {
		fail(w, domain.Err("unauthenticated", "local credential or session required"))
		return
	}
	if s.CSRF != "" && r.Method != "GET" && r.Method != "HEAD" && (!h.originOK(origin) || !equal(r.Header.Get("X-CSRF-Token"), s.CSRF)) {
		fail(w, domain.Err("forbidden", "Origin and CSRF token required"))
		return
	}
	if r.URL.Path == "/api/v1/config" && r.Method == "GET" {
		write(w, 200, map[string]any{"timezone": h.opts.Timezone})
		return
	}
	h.tasks(w, r)
}
func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		if !h.bearer(r) {
			fail(w, domain.Err("unauthenticated", "bearer credential required"))
			return
		}
		h.mu.Lock()
		for id, s := range h.sessions {
			if !time.Now().Before(s.Expires) {
				delete(h.sessions, id)
			}
		}
		if len(h.sessions) >= 256 {
			h.mu.Unlock()
			fail(w, domain.Err("store_busy", "too many browser sessions"))
			return
		}
		id := secret()
		s := session{CSRF: secret(), Expires: time.Now().Add(8 * time.Hour)}
		h.sessions[id] = s
		h.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: id, Path: "/api/v1", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 8 * 60 * 60})
		write(w, 200, map[string]any{"csrf_token": s.CSRF, "expires_at": s.Expires})
	case "GET", "DELETE":
		s, ok := h.lookup(r)
		if !ok {
			fail(w, domain.Err("unauthenticated", "browser session required"))
			return
		}
		if r.Method == "GET" {
			write(w, 200, map[string]any{"csrf_token": s.CSRF, "expires_at": s.Expires})
			return
		}
		if !h.originOK(r.Header.Get("Origin")) || !equal(r.Header.Get("X-CSRF-Token"), s.CSRF) {
			fail(w, domain.Err("forbidden", "Origin and CSRF token required"))
			return
		}
		c, _ := r.Cookie(cookieName)
		h.mu.Lock()
		delete(h.sessions, c.Value)
		h.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/api/v1", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		write(w, 200, map[string]bool{"logged_out": true})
	default:
		fail(w, domain.Err("method_not_allowed", "unsupported session method"))
	}
}

type mutation struct {
	OperationID      string          `json:"operation_id"`
	Actor            string          `json:"actor"`
	ExpectedRevision *int64          `json:"expected_revision,omitempty"`
	ClaimToken       string          `json:"claim_token,omitempty"`
	Override         bool            `json:"override,omitempty"`
	Data             json.RawMessage `json:"data"`
}

func (h *Handler) tasks(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	if len(parts) == 0 || parts[0] != "tasks" {
		fail(w, domain.Err("not_found", "unknown API route"))
		return
	}
	kind, id := "", ""
	if len(parts) == 1 {
		switch r.Method {
		case "GET":
			values := r.URL.Query()
			if len(values.Get("from")) == 10 || len(values.Get("to")) == 10 {
				a, b, e := domain.DateWindow(values.Get("from"), values.Get("to"), h.opts.Timezone)
				if e != nil {
					fail(w, e)
					return
				}
				if len(values["from"]) != 1 || len(values["to"]) != 1 {
					fail(w, domain.Err("invalid_input", "duplicate date parameters"))
					return
				}
				values.Set("from", a.Format(time.RFC3339))
				values.Set("to", b.Format(time.RFC3339))
			}
			q, e := query(values)
			if e != nil {
				fail(w, e)
				return
			}
			v, e := h.svc.List(r.Context(), q)
			if e != nil {
				fail(w, e)
			} else {
				write(w, 200, v)
			}
			return
		case "POST":
			kind = "create"
		default:
			fail(w, domain.Err("method_not_allowed", "use GET or POST"))
			return
		}
	} else {
		id = parts[1]
		if !domain.ValidID(id) {
			fail(w, domain.Err("invalid_input", "invalid task ID"))
			return
		}
		suffix := strings.Join(parts[2:], "/")
		if r.Method == "GET" {
			var v any
			var e error
			switch suffix {
			case "":
				b, err := h.svc.Get(r.Context(), id)
				v, e = b.Task, err
			case "context":
				offset, limit := 0, 10
				var expected *int64
				for k, vs := range r.URL.Query() {
					if len(vs) != 1 || (k != "offset" && k != "limit" && k != "expected_revision") {
						e = domain.Err("invalid_input", "invalid context query")
						break
					}
					n, err := strconv.ParseInt(vs[0], 10, 64)
					if err != nil {
						e = domain.Err("invalid_input", "invalid context range")
						break
					}
					switch k {
					case "offset":
						offset = int(n)
					case "limit":
						limit = int(n)
					case "expected_revision":
						expected = &n
					}
				}
				if e == nil {
					v, e = h.svc.ContextPage(r.Context(), id, offset, limit, expected)
				}
			case "history":
				offset, limit := 0, 50
				values := r.URL.Query()
				for k, vs := range values {
					if len(vs) != 1 || (k != "offset" && k != "limit") {
						e = domain.Err("invalid_input", "invalid history query")
						break
					}
					n, err := strconv.Atoi(vs[0])
					if err != nil {
						e = domain.Err("invalid_input", "invalid history range")
						break
					}
					if k == "offset" {
						offset = n
					} else {
						limit = n
					}
				}
				if e == nil {
					v, e = h.svc.History(r.Context(), id, offset, limit)
				}
			default:
				e = domain.Err("not_found", "unknown task route")
			}
			if e != nil {
				fail(w, e)
			} else {
				write(w, 200, v)
			}
			return
		}
		if suffix == "" && r.Method == "PATCH" {
			kind = "update"
		} else if r.Method == "POST" {
			kind = map[string]string{"claim": "claim", "claim/renew": "renew", "claim/release": "release", "reports": "report", "checks": "check", "observations": "observe"}[suffix]
		}
		if kind == "" {
			fail(w, domain.Err("method_not_allowed", "unsupported task operation"))
			return
		}
	}
	var in mutation
	if e := body(w, r, &in); e != nil {
		fail(w, e)
		return
	}
	v, e := h.svc.Execute(r.Context(), app.Request{Kind: kind, TaskID: id, OperationID: in.OperationID, Actor: in.Actor, ExpectedRevision: in.ExpectedRevision, ClaimToken: in.ClaimToken, Override: in.Override, Data: in.Data})
	if e != nil {
		fail(w, e)
	} else {
		write(w, 200, v)
	}
}
func query(values url.Values) (domain.Query, error) {
	obj := map[string]any{}
	t := reflect.TypeFor[domain.Query]()
	fields := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fields[strings.Split(f.Tag.Get("json"), ",")[0]] = f.Type
	}
	for k, vs := range values {
		typ, ok := fields[k]
		if !ok {
			return domain.Query{}, domain.Err("invalid_input", "unknown query field: "+k)
		}
		if k == "statuses" {
			obj[k] = vs
			continue
		}
		if len(vs) != 1 {
			return domain.Query{}, domain.Err("invalid_input", "duplicate query field")
		}
		if typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		switch typ.Kind() {
		case reflect.Bool:
			v, e := strconv.ParseBool(vs[0])
			if e != nil {
				return domain.Query{}, domain.Err("invalid_input", "invalid boolean")
			}
			obj[k] = v
		case reflect.Int:
			v, e := strconv.Atoi(vs[0])
			if e != nil {
				return domain.Query{}, domain.Err("invalid_input", "invalid integer")
			}
			obj[k] = v
		default:
			obj[k] = vs[0]
		}
	}
	b, _ := json.Marshal(obj)
	var q domain.Query
	e := app.Decode(b, &q)
	return q, e
}

func Serve(ctx context.Context, svc app.Service, address, token, devOrigin, timezone string, log io.Writer, ui http.Handler) error {
	host, _, e := net.SplitHostPort(address)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return domain.Err("invalid_input", "listen address must be a loopback IP and port")
	}
	l, e := net.Listen("tcp", address)
	if e != nil {
		return e
	}
	defer l.Close()
	h, e := New(svc, Options{Host: l.Addr().String(), Token: token, DevOrigin: devOrigin, Timezone: timezone, UI: ui})
	if e != nil {
		return e
	}
	server := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		case <-done:
		}
	}()
	fmt.Fprintln(log, "Movement API: http://"+l.Addr().String())
	e = server.Serve(l)
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
