//go:build darwin

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	httpapi "movement.local/movement/internal/adapters/http"
	mcpadapter "movement.local/movement/internal/adapters/mcp"
	"movement.local/movement/internal/app"
	"movement.local/movement/internal/config"
	"movement.local/movement/internal/domain"
	store "movement.local/movement/internal/store/text"
	"movement.local/movement/internal/web"
)

const Help = `Movement — local tasks for people and agents

Usage:
  movement [--data-dir PATH] init [--timezone Europe/Paris]
  movement [--data-dir PATH] task create --input task.json --op ID [--actor NAME]
  movement [--data-dir PATH] task list [--query query.json] [--json]
  movement [--data-dir PATH] task get|context|history TASK_ID [--json]
  movement [--data-dir PATH] task update|claim|renew|release|report|check-report|observe TASK_ID --revision N --op ID [--input payload.json] [--claim-token TOKEN] [--override]
  movement [--data-dir PATH] request --input request.json
  movement [--data-dir PATH] mcp --stdio
  movement [--data-dir PATH] serve [--listen 127.0.0.1:8484] [--dev-origin http://localhost:5173]
  movement [--data-dir PATH] auth token|rotate
  movement [--data-dir PATH] verify

Use --input - or --query - to read JSON from stdin. Mutations print receipts as JSON.
--data-dir overrides MOVEMENT_DATA_DIR, then defaults to ~/.movement.
Global flags must precede the command; task flags follow the task ID.
Execution/check guidance is passed to external assistants; Movement never runs it.
`

func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	err := run(ctx, args, in, out, errOut)
	if err == nil {
		return 0
	}
	var de *domain.Error
	if !errors.As(err, &de) {
		de = &domain.Error{Code: "internal_error", Message: err.Error()}
	}
	_ = json.NewEncoder(errOut).Encode(map[string]any{"error": de})
	switch de.Code {
	case "invalid_input":
		return 2
	case "not_found":
		return 3
	case "revision_conflict", "context_conflict", "assessment_conflict", "operation_conflict", "claim_conflict", "invalid_transition":
		return 4
	case "store_busy":
		return 5
	default:
		return 1
	}
}
func flags(name string, w io.Writer) *flag.FlagSet {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(w)
	return f
}
func readInput(path string, in io.Reader) ([]byte, error) {
	if path == "" {
		return []byte(`{}`), nil
	}
	var r io.Reader = in
	if path != "-" {
		f, e := os.Open(path)
		if e != nil {
			return nil, domain.Err("invalid_input", e.Error())
		}
		defer f.Close()
		r = f
	}
	b, e := io.ReadAll(io.LimitReader(r, 1024*1024+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 1024*1024 {
		return nil, domain.Err("invalid_input", "input exceeds 1 MiB")
	}
	return b, nil
}
func output(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
func run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, e := io.WriteString(out, Help)
		return e
	}
	global := flags("movement", errOut)
	path := global.String("data-dir", "", "local task directory")
	if e := global.Parse(args); e != nil {
		return domain.Err("invalid_input", e.Error())
	}
	args = global.Args()
	if len(args) == 0 {
		return domain.Err("invalid_input", "missing command")
	}
	dir, e := config.DataDir(*path)
	if e != nil {
		return e
	}
	repo := store.New(dir)
	svc := app.Service{Repo: repo}
	if args[0] == "init" {
		f := flags("init", errOut)
		zone := f.String("timezone", "UTC", "IANA timezone")
		if e = f.Parse(args[1:]); e != nil {
			return domain.Err("invalid_input", e.Error())
		}
		if f.NArg() != 0 {
			return domain.Err("invalid_input", "unexpected init arguments")
		}
		c, e := repo.Initialize(ctx, *zone)
		if e != nil {
			return e
		}
		return output(out, map[string]any{"data_dir": dir, "config": c})
	}
	settings, e := repo.Settings(ctx)
	if e != nil {
		return e
	}
	if args[0] == "verify" && len(args) == 1 {
		verified, err := repo.Verify(ctx)
		if err != nil {
			return err
		}
		return output(out, verified)
	}
	if args[0] == "auth" && len(args) == 2 && args[1] == "rotate" {
		if err := repo.RotateAPIToken(ctx); err != nil {
			return err
		}
		return output(out, map[string]any{"rotated": true, "notice": "Restart all HTTP servers to invalidate their old credentials and browser sessions. Read the new token with auth token."})
	}
	if args[0] == "auth" && len(args) == 2 && args[1] == "token" {
		token, e := repo.APIToken(ctx)
		if e != nil {
			return e
		}
		_, e = fmt.Fprintln(out, token)
		return e
	}
	if args[0] == "serve" {
		f := flags("serve", errOut)
		address := f.String("listen", "127.0.0.1:8484", "loopback IP and port")
		origin := f.String("dev-origin", "", "exact Vite origin")
		if e = f.Parse(args[1:]); e != nil {
			return domain.Err("invalid_input", e.Error())
		}
		if f.NArg() != 0 {
			return domain.Err("invalid_input", "unexpected serve arguments")
		}
		token, e := repo.APIToken(ctx)
		if e != nil {
			return e
		}
		return httpapi.Serve(ctx, svc, *address, token, *origin, settings.Timezone, errOut, web.Handler())
	}
	if args[0] == "mcp" {
		if len(args) != 2 || args[1] != "--stdio" {
			return domain.Err("invalid_input", "use mcp --stdio")
		}
		return mcpadapter.Run(ctx, svc, in, out)
	}
	if args[0] == "request" {
		f := flags("request", errOut)
		input := f.String("input", "-", "JSON request file or stdin")
		if e = f.Parse(args[1:]); e != nil {
			return domain.Err("invalid_input", e.Error())
		}
		raw, e := readInput(*input, in)
		if e != nil {
			return e
		}
		var req app.Request
		if e = app.Decode(raw, &req); e != nil {
			return e
		}
		receipt, e := svc.Execute(ctx, req)
		if e != nil {
			return e
		}
		return output(out, receipt)
	}
	if args[0] != "task" || len(args) < 2 {
		return domain.Err("invalid_input", "unknown command; use movement help")
	}
	kind := args[1]
	rest := args[2:]
	id := ""
	if kind != "create" && kind != "list" {
		if len(rest) == 0 {
			return domain.Err("invalid_input", "task ID required")
		}
		id = rest[0]
		rest = rest[1:]
	}
	f := flags(kind, errOut)
	input := f.String("input", "", "JSON payload file or -")
	query := f.String("query", "", "JSON query file or -")
	op := f.String("op", "", "stable operation ID, required for mutation retries")
	actor := f.String("actor", "user", "actor label")
	revision := f.String("revision", "", "expected task revision")
	token := f.String("claim-token", "", "execution claim token")
	override := f.Bool("override", false, "explicitly override current execution claim")
	asJSON := f.Bool("json", false, "JSON output for read commands")
	offset := f.Int("offset", 0, "history offset")
	defaultLimit := 50
	if kind == "context" {
		defaultLimit = 10
	}
	limit := f.Int("limit", defaultLimit, "page size")
	from := f.String("from", "", "inclusive RFC3339 instant or date")
	to := f.String("to", "", "exclusive RFC3339 instant or date")
	flow := f.String("flow", "", "actionable, waiting_for, all")
	view := f.String("view", "", "planning or execution")
	if e = f.Parse(rest); e != nil {
		return domain.Err("invalid_input", e.Error())
	}
	if f.NArg() != 0 {
		return domain.Err("invalid_input", "unexpected arguments")
	}
	switch kind {
	case "get":
		b, e := svc.Get(ctx, id)
		if e != nil {
			return e
		}
		if *asJSON {
			return output(out, b.Task)
		}
		_, e = fmt.Fprintf(out, "%s  %s\nStatus: %s · Flow: %s · Revision: %d\n%s\n", b.Task.ID, b.Task.Title, b.Task.Status, b.Task.Flow, b.Task.Revision, b.Task.Description)
		return e
	case "context":
		var expected *int64
		if *revision != "" {
			v, e := strconv.ParseInt(*revision, 10, 64)
			if e != nil {
				return domain.Err("invalid_input", "invalid revision")
			}
			expected = &v
		}
		p, e := svc.ContextPage(ctx, id, *offset, *limit, expected)
		if e != nil {
			return e
		}
		return output(out, p)
	case "history":
		events, e := svc.History(ctx, id, *offset, *limit)
		if e != nil {
			return e
		}
		return output(out, events)
	case "list":
		raw, e := readInput(*query, in)
		if e != nil {
			return e
		}
		var q domain.Query
		if e = app.Decode(raw, &q); e != nil {
			return e
		}
		if *flow != "" {
			q.Flow = *flow
		}
		if *view != "" {
			q.View = *view
		}
		if q.Limit == 0 {
			q.Limit = *limit
		}
		if *from != "" || *to != "" {
			var a, b time.Time
			if len(*from) == 10 && len(*to) == 10 {
				a, b, e = domain.DateWindow(*from, *to, settings.Timezone)
			} else {
				a, e = time.Parse(time.RFC3339, *from)
				if e == nil {
					b, e = time.Parse(time.RFC3339, *to)
				}
			}
			if e != nil {
				return domain.Err("invalid_input", e.Error())
			}
			q.From = &a
			q.To = &b
		}
		p, e := svc.List(ctx, q)
		if e != nil {
			return e
		}
		if *asJSON {
			return output(out, p)
		}
		for _, item := range p.Items {
			if _, e = fmt.Fprintf(out, "%s  %-16s %s\n", item.Task.ID, item.Task.Status, item.Task.Title); e != nil {
				return e
			}
		}
		if p.NextCursor != "" {
			_, e = fmt.Fprintln(out, "More results available; use --json and the returned cursor.")
		}
		return e
	}
	if kind == "check-report" {
		kind = "check"
	}
	if !strings.Contains("|create|update|claim|renew|release|report|check|observe|", "|"+kind+"|") {
		return domain.Err("invalid_input", "unknown task command")
	}
	raw, e := readInput(*input, in)
	if e != nil {
		return e
	}
	req := app.Request{OperationID: *op, Kind: kind, TaskID: id, Actor: *actor, ClaimToken: *token, Override: *override, Data: raw}
	if *revision != "" {
		v, e := strconv.ParseInt(*revision, 10, 64)
		if e != nil {
			return domain.Err("invalid_input", "invalid revision")
		}
		req.ExpectedRevision = &v
	}
	receipt, e := svc.Execute(ctx, req)
	if e != nil {
		return e
	}
	return output(out, receipt)
}
