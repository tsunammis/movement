//go:build darwin

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"movement.local/movement/internal/app"
	"strings"
	"testing"
)

func TestCLIRoundTrip(t *testing.T) {
	dir := t.TempDir()
	run := func(input string, args ...string) (string, string, int) {
		var out, err bytes.Buffer
		status := Run(context.Background(), append([]string{"--data-dir", dir}, args...), strings.NewReader(input), &out, &err)
		return out.String(), err.String(), status
	}
	if _, e, c := run("", "init", "--timezone", "Europe/Paris"); c != 0 {
		t.Fatal(e)
	}
	out, e, c := run(`{"title":"Draft email","status":"ready"}`, "task", "create", "--op", "create-1", "--input", "-")
	if c != 0 {
		t.Fatal(e)
	}
	var receipt app.Receipt
	if err := json.Unmarshal([]byte(out), &receipt); err != nil {
		t.Fatal(err)
	}
	if _, e, c = run("", "task", "get", receipt.TaskID, "--json"); c != 0 {
		t.Fatal(e)
	}
	if _, e, c = run(`{"fields":{"title":"Edited"}}`, "task", "update", receipt.TaskID, "--revision", "0", "--op", "edit-1", "--input", "-"); c != 4 || !strings.Contains(e, "revision_conflict") {
		t.Fatalf("wanted revision conflict %d %s", c, e)
	}
	if _, e, c = run(`{"fields":{"title":"Edited"}}`, "task", "update", receipt.TaskID, "--revision", "1", "--op", "edit-1", "--input", "-"); c != 0 {
		t.Fatal(e)
	}
	out, e, c = run("", "task", "list", "--json")
	if c != 0 || !strings.Contains(out, "Edited") {
		t.Fatalf("list %s %s", out, e)
	}
	if _, e, c = run("", "task", "get", "not-a-uuid"); c != 2 {
		t.Fatalf("wanted invalid input %d %s", c, e)
	}
}
func TestCLIRequiresInitialization(t *testing.T) {
	var out, err bytes.Buffer
	status := Run(context.Background(), []string{"--data-dir", t.TempDir(), "task", "list"}, strings.NewReader(""), &out, &err)
	if status != 1 || !strings.Contains(err.String(), "not_initialized") {
		t.Fatalf("%d %s", status, err.String())
	}
}

func TestCLIVerifyAndRotate(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		var out, err bytes.Buffer
		if code := Run(context.Background(), append([]string{"--data-dir", dir}, args...), strings.NewReader(""), &out, &err); code != 0 {
			t.Fatalf("%v: %s", args, err.String())
		}
		return out.String()
	}
	run("init")
	before := run("auth", "token")
	if output := run("verify"); !strings.Contains(output, `"tasks": 0`) {
		t.Fatal(output)
	}
	rotated := run("auth", "rotate")
	if strings.Contains(rotated, strings.TrimSpace(before)) || !strings.Contains(rotated, `"rotated": true`) {
		t.Fatal("rotation output")
	}
	if after := run("auth", "token"); before == after {
		t.Fatal("token unchanged")
	}
	run("verify")
}
