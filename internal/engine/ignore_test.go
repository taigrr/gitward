package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taigrr/gitward/internal/leaf"
	"github.com/taigrr/gitward/internal/store"
)

// TestIgnore_NotCapturedAndPreserved covers the core contract: an ignored
// variable present in a leaf is never captured into the store, survives
// store->leaf regeneration in the trailing ignored block, and is not reported
// as drift.
func TestIgnore_NotCapturedAndPreserved(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/web", "_", store.Runtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"TOKEN": "abc"}); err != nil {
		t.Fatal(err)
	}

	// Ignore a platform-injected var for this app/env/tier.
	if err := e.Ignore(cell, "DATABASE_URL"); err != nil {
		t.Fatal(err)
	}

	// Dev has the ignored var locally alongside the managed one.
	leafPath := filepath.Join(dir, "apps/web/.dev.vars")
	writeFile(t, leafPath, "TOKEN=abc\nDATABASE_URL=postgres://local\n")

	// Sync: no conflicts, DATABASE_URL never enters the store.
	plan, err := e.Plan()
	if err != nil {
		t.Fatal(err)
	}
	for _, cr := range plan {
		for _, r := range cr.Results {
			if r.Key == "DATABASE_URL" {
				t.Fatalf("ignored key surfaced in plan: %+v", r)
			}
		}
	}
	if _, err := e.Apply(plan, Both); err != nil {
		t.Fatal(err)
	}
	sp, _ := e.DecryptStore()
	if _, ok := sp["apps/web"]["_"][store.Runtime]["DATABASE_URL"]; ok {
		t.Fatal("ignored var was captured into the store")
	}

	// Regenerate the leaf from the store; ignored var must survive verbatim in
	// the ignored block.
	if err := os.Remove(leafPath); err != nil {
		t.Fatal(err)
	}
	writeFile(t, leafPath, "TOKEN=abc\nDATABASE_URL=postgres://local\n")
	plan, _ = e.Plan()
	if _, err := e.Apply(plan, StoreToLeaf); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, leafPath)
	if !strings.Contains(got, leaf.IgnoredHeader) {
		t.Fatalf("ignored header missing:\n%s", got)
	}
	if !strings.Contains(got, "DATABASE_URL=postgres://local") {
		t.Fatalf("ignored var dropped on regeneration:\n%s", got)
	}
}

// TestIgnore_EvictsExistingNoLeaf verifies that ignoring an already-stored
// variable preserves its value in the regenerated leaf even when no leaf file
// exists on disk at ignore time (fresh clone / cleaned tree).
func TestIgnore_EvictsExistingNoLeaf(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/api", "_", store.Buildtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"KEEP": "1", "DROP": "2"}); err != nil {
		t.Fatal(err)
	}
	// Remove the leaf so eviction cannot recover the value from disk.
	if err := os.Remove(filepath.Join(dir, "apps/api/.env")); err != nil {
		t.Fatal(err)
	}

	if err := e.Ignore(cell, "DROP"); err != nil {
		t.Fatal(err)
	}
	sp, _ := e.DecryptStore()
	if _, ok := sp["apps/api"]["_"][store.Buildtime]["DROP"]; ok {
		t.Fatal("DROP not evicted from store")
	}
	leafBody := readFile(t, filepath.Join(dir, "apps/api/.env"))
	if !strings.Contains(leafBody, leaf.IgnoredHeader) || !strings.Contains(leafBody, "DROP=2") {
		t.Fatalf("evicted value lost when leaf absent:\n%s", leafBody)
	}
}

// TestIgnore_PreservesLocalManagedEdits verifies that ignoring one key does not
// revert unsynced local edits to other managed keys in the same leaf.
func TestIgnore_PreservesLocalManagedEdits(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/api", "_", store.Buildtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"TOKEN": "old", "DROP": "2"}); err != nil {
		t.Fatal(err)
	}
	// Local, unsynced edit to a managed key.
	writeFile(t, filepath.Join(dir, "apps/api/.env"), "TOKEN=new\nDROP=2\n")

	if err := e.Ignore(cell, "DROP"); err != nil {
		t.Fatal(err)
	}
	body := readFile(t, filepath.Join(dir, "apps/api/.env"))
	if !strings.Contains(body, "TOKEN=new") {
		t.Fatalf("local managed edit clobbered:\n%s", body)
	}
	if !strings.Contains(body, "DROP=2") {
		t.Fatalf("ignored var lost:\n%s", body)
	}
}

// TestWriteLeaf_DoesNotEmitIgnoredAsManaged verifies that an ignore marker is
// enough to suppress a managed value, even when there is no ignored value to
// preserve in the trailing block.
func TestWriteLeaf_DoesNotEmitIgnoredAsManaged(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/api", "_", store.Buildtime}
	e.setIgnoreMarker(cell, "SECRET")

	if err := e.writeLeaf(cell.Path, cell.Tier, cell.Env, map[string]string{
		"KEEP":   "1",
		"SECRET": "2",
	}, nil); err != nil {
		t.Fatal(err)
	}
	body := readFile(t, filepath.Join(dir, "apps/api/.env"))
	if strings.Contains(body, "SECRET=") {
		t.Fatalf("ignored key emitted as managed:\n%s", body)
	}
	if !strings.Contains(body, "KEEP=1") {
		t.Fatalf("managed key missing:\n%s", body)
	}
}

// TestIgnore_EvictsExisting verifies that ignoring an already-stored variable
// drops it from the store and base but keeps it in the leaf's ignored block.
func TestIgnore_EvictsExisting(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/api", "_", store.Buildtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"KEEP": "1", "DROP": "2"}); err != nil {
		t.Fatal(err)
	}

	if err := e.Ignore(cell, "DROP"); err != nil {
		t.Fatal(err)
	}

	sp, _ := e.DecryptStore()
	got := sp["apps/api"]["_"][store.Buildtime]
	if _, ok := got["DROP"]; ok {
		t.Fatalf("DROP not evicted from store: %#v", got)
	}
	if got["KEEP"] != "1" {
		t.Fatalf("KEEP lost: %#v", got)
	}
	base, _ := e.LoadBase()
	if _, ok := base["apps/api"]["_"][store.Buildtime]["DROP"]; ok {
		t.Fatal("DROP not evicted from base")
	}
	leafBody := readFile(t, filepath.Join(dir, "apps/api/.env"))
	if !strings.Contains(leafBody, leaf.IgnoredHeader) || !strings.Contains(leafBody, "DROP=2") {
		t.Fatalf("DROP should remain in the ignored block:\n%s", leafBody)
	}
}

// TestIgnore_RegisterSkips ensures scanning/registering never picks up ignored
// variables.
func TestIgnore_RegisterSkips(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/x", "_", store.Buildtime}
	if err := e.Ignore(cell, "IGNORED"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "apps/x/.env"), "REAL=1\nIGNORED=2\n")
	n, err := e.Register("")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("registered %d keys, want 1 (IGNORED must be skipped)", n)
	}
	sp, _ := e.DecryptStore()
	if _, ok := sp["apps/x"]["_"][store.Buildtime]["IGNORED"]; ok {
		t.Fatal("ignored var was registered")
	}
}

// TestIgnore_PerEnvIsolation verifies ignoring a key in one env does not affect
// the same key in another env of the same target/tier.
func TestIgnore_PerEnvIsolation(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	dev := Cell{"apps/web", "development", store.Buildtime}
	prod := Cell{"apps/web", "production", store.Buildtime}
	if err := e.SetCellPlaintext(dev, map[string]string{"API": "d"}); err != nil {
		t.Fatal(err)
	}
	if err := e.SetCellPlaintext(prod, map[string]string{"API": "p"}); err != nil {
		t.Fatal(err)
	}

	if err := e.Ignore(dev, "API"); err != nil {
		t.Fatal(err)
	}
	if !e.Store.Ignored("apps/web", "development", store.Buildtime, "API") {
		t.Fatal("API should be ignored in development")
	}
	if e.Store.Ignored("apps/web", "production", store.Buildtime, "API") {
		t.Fatal("API must NOT be ignored in production")
	}
	sp, _ := e.DecryptStore()
	if _, ok := sp["apps/web"]["development"][store.Buildtime]["API"]; ok {
		t.Fatal("API not evicted from development store")
	}
	if sp["apps/web"]["production"][store.Buildtime]["API"] != "p" {
		t.Fatal("production API must remain managed")
	}
}

// TestDoctor_DetectsMalformedEntry ensures the store-entry check flags an entry
// that has both enc and ignore set.
func TestDoctor_DetectsMalformedEntry(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/api", "_", store.Buildtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"K": "v"}); err != nil {
		t.Fatal(err)
	}
	// Corrupt the store: mark a managed (ciphertext-bearing) entry as ignored.
	ev := e.Store.Targets["apps/api"]["_"].Buildtime["K"]
	ev.Ignore = true
	e.Store.Targets["apps/api"]["_"].Buildtime["K"] = ev

	var found bool
	for _, c := range e.Doctor() {
		if c.Name == "store entries" {
			found = true
			if c.OK {
				t.Fatalf("store entries check passed on malformed entry: %q", c.Info)
			}
			if !strings.Contains(c.Info, "both enc and ignore") {
				t.Fatalf("unexpected info: %q", c.Info)
			}
		}
	}
	if !found {
		t.Fatal("no 'store entries' check produced")
	}
}

// TestUnignore_ResumesManagement verifies a variable is captured again after
// being unignored.
func TestUnignore_ResumesManagement(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/x", "_", store.Buildtime}
	if err := e.Ignore(cell, "V"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "apps/x/.env"), "V=1\n")

	if err := e.Unignore(cell, "V"); err != nil {
		t.Fatal(err)
	}
	if e.Store.Ignored("apps/x", "_", store.Buildtime, "V") {
		t.Fatal("V still ignored after unignore")
	}
	if _, err := e.Register(""); err != nil {
		t.Fatal(err)
	}
	sp, _ := e.DecryptStore()
	if sp["apps/x"]["_"][store.Buildtime]["V"] != "1" {
		t.Fatalf("V not captured after unignore: %#v", sp["apps/x"]["_"][store.Buildtime])
	}
}
