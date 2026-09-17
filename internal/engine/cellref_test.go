package engine

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taigrr/gitward/internal/store"
)

func TestParseCellFile(t *testing.T) {
	cases := []struct {
		in   string
		want Cell
		err  bool
	}{
		{"apps/web/.env", Cell{"apps/web", "_", store.Buildtime}, false},
		{"apps/web/.env.production", Cell{"apps/web", "production", store.Buildtime}, false},
		{"svc/.dev.vars", Cell{"svc", "_", store.Runtime}, false},
		{"svc/.dev.vars.staging", Cell{"svc", "staging", store.Runtime}, false},
		{".env", Cell{"", "_", store.Buildtime}, false},
		{"apps/web/.env.example", Cell{}, true},
		{"apps/web/config.json", Cell{}, true},
	}
	for _, tc := range cases {
		got, err := ParseCellFile(tc.in)
		if (err != nil) != tc.err {
			t.Errorf("%s: err=%v want err=%v", tc.in, err, tc.err)
			continue
		}
		if !tc.err && got != tc.want {
			t.Errorf("%s: got %+v want %+v", tc.in, got, tc.want)
		}
		if !tc.err && got.File() != tc.in {
			t.Errorf("%s: File() round-trip = %q", tc.in, got.File())
		}
	}
	if IsLeafFileName("foo/.env.example") || !IsLeafFileName("foo/.dev.vars.x") {
		t.Error("IsLeafFileName misclassified")
	}
}

func TestRegisterPlan_ReportsWithoutWriting(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/web", "production", store.Buildtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"A": "1"}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "apps/web/.env.production"), "A=1\nB=2\nC=3\n")
	writeFile(t, filepath.Join(dir, "svc/.dev.vars"), "K=v\n")

	entries, err := e.RegisterPlan("")
	if err != nil {
		t.Fatal(err)
	}
	want := []RegisterEntry{
		{Cell: cell, Keys: []string{"B", "C"}},
		{Cell: Cell{"svc", "_", store.Runtime}, Keys: []string{"K"}},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("RegisterPlan = %+v, want %+v", entries, want)
	}
	// Nothing was written.
	sp, err := e.DecryptStore()
	if err != nil {
		t.Fatal(err)
	}
	if got := cellVals(sp, cell); len(got) != 1 {
		t.Fatalf("RegisterPlan mutated store: %v", got)
	}
	n, err := e.Register("")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("Register = %d, want 3", n)
	}
	entries, err = e.RegisterPlan("")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("RegisterPlan after Register = %+v, want empty", entries)
	}
}

func TestSetKeysUnsetKeys(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"svc", "_", store.Runtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"A": "1", "B": "2"}); err != nil {
		t.Fatal(err)
	}
	if err := e.SetKeys(cell, map[string]string{"B": "changed", "C": "3"}); err != nil {
		t.Fatal(err)
	}
	vals, err := e.CellPlaintext(cell)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "B": "changed", "C": "3"}
	if !reflect.DeepEqual(vals, want) {
		t.Fatalf("after SetKeys = %v, want %v", vals, want)
	}
	// Leaf and base advanced: plan is clean.
	plan, err := e.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if CellDirty(plan, cell) {
		t.Fatal("SetKeys left the cell dirty")
	}

	removed, err := e.UnsetKeys(cell, []string{"A", "MISSING"})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("UnsetKeys removed %d, want 1", removed)
	}
	vals, _ = e.CellPlaintext(cell)
	if _, ok := vals["A"]; ok {
		t.Fatal("A still present after UnsetKeys")
	}
	leafContent := readFile(t, filepath.Join(dir, "svc/.dev.vars"))
	if strings.Contains(leafContent, "A=1") || !strings.Contains(leafContent, "B=changed") {
		t.Fatalf("leaf not regenerated: %q", leafContent)
	}
	plan, _ = e.Plan()
	if CellDirty(plan, cell) {
		t.Fatal("UnsetKeys left the cell dirty")
	}
}

func TestCellDirty(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"svc", "_", store.Runtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"A": "1"}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "svc/.dev.vars"), "A=edited\n")
	plan, err := e.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if !CellDirty(plan, cell) {
		t.Fatal("expected dirty cell after local edit")
	}
	if CellDirty(plan, Cell{"other", "_", store.Runtime}) {
		t.Fatal("unknown cell reported dirty")
	}
}
