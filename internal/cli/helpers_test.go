package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taigrr/gitward/internal/engine"
	"github.com/taigrr/gitward/internal/store"
)

func TestParseCellRef(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args     []string
		wantCell engine.Cell
		wantRest []string
		err      bool
	}{
		{[]string{"apps/web/.env.production", "KEY"}, engine.Cell{Path: "apps/web", Env: "production", Tier: store.Buildtime}, []string{"KEY"}, false},
		{[]string{"apps/web", "production", "buildtime", "KEY"}, engine.Cell{Path: "apps/web", Env: "production", Tier: store.Buildtime}, []string{"KEY"}, false},
		{[]string{"svc", "_", "runtime"}, engine.Cell{Path: "svc", Env: "_", Tier: store.Runtime}, []string{}, false},
		{[]string{"svc/.dev.vars"}, engine.Cell{Path: "svc", Env: "_", Tier: store.Runtime}, []string{}, false},
		{[]string{"svc", "_", "nope"}, engine.Cell{}, nil, true},
		{[]string{"svc", "_"}, engine.Cell{}, nil, true},
		{[]string{"../outside/.env"}, engine.Cell{}, nil, true},
		{nil, engine.Cell{}, nil, true},
	}
	for _, tc := range cases {
		cell, rest, err := parseCellRef(root, tc.args)
		if (err != nil) != tc.err {
			t.Errorf("%v: err=%v want err=%v", tc.args, err, tc.err)
			continue
		}
		if tc.err {
			continue
		}
		if cell != tc.wantCell {
			t.Errorf("%v: cell=%+v want %+v", tc.args, cell, tc.wantCell)
		}
		if len(rest) != len(tc.wantRest) || (len(rest) > 0 && !reflect.DeepEqual(rest, tc.wantRest)) {
			t.Errorf("%v: rest=%v want %v", tc.args, rest, tc.wantRest)
		}
	}
}

func TestCellArgCount(t *testing.T) {
	cases := []struct {
		min, max int
		args     []string
		ok       bool
	}{
		{0, 0, []string{"a/.env"}, true},
		{0, 0, []string{"a", "_", "buildtime"}, true},
		{0, 0, []string{"a/.env", "K"}, false},
		{1, 1, []string{"a/.env", "K"}, true},
		{1, 1, []string{"a", "_", "buildtime", "K"}, true},
		{1, 1, []string{"a", "_", "buildtime"}, false},
		{1, -1, []string{"a/.env", "K=1", "J=2", "L=3"}, true},
		{0, 1, nil, false},
	}
	for _, tc := range cases {
		err := cellArgCount(tc.min, tc.max)(nil, tc.args)
		if (err == nil) != tc.ok {
			t.Errorf("cellArgCount(%d,%d)(%v) err=%v want ok=%v", tc.min, tc.max, tc.args, err, tc.ok)
		}
	}
}

func TestParseAssignments(t *testing.T) {
	got, err := parseAssignments([]string{"A=1", "B=x=y", "C="}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "B": "x=y", "C": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err := parseAssignments([]string{"NOEQ"}, false, nil); err == nil {
		t.Fatal("expected error for bare key without --stdin")
	}
	if _, err := parseAssignments([]string{"=v"}, false, nil); err == nil {
		t.Fatal("expected error for empty key")
	}
	got, err = parseAssignments([]string{"K"}, true, strings.NewReader("multi word secret\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got["K"] != "multi word secret" {
		t.Fatalf("stdin value = %q", got["K"])
	}
	if _, err := parseAssignments([]string{"K=1"}, true, strings.NewReader("")); err == nil {
		t.Fatal("expected error for KEY=VALUE with --stdin")
	}
	if _, err := parseAssignments([]string{"K", "J"}, true, strings.NewReader("")); err == nil {
		t.Fatal("expected error for multiple keys with --stdin")
	}
}

func TestExitCode(t *testing.T) {
	if ExitCode(nil) != ExitOK {
		t.Fatal("nil should be ExitOK")
	}
	if ExitCode(errNotInitialized) != ExitFailure {
		t.Fatal("plain error should be ExitFailure")
	}
	if ExitCode(&ExitError{Code: ExitConflict}) != ExitConflict {
		t.Fatal("ExitError code not propagated")
	}
}
