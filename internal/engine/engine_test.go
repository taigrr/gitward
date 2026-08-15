package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/taigrr/gitward/internal/crypto"
	"github.com/taigrr/gitward/internal/store"
)

const pass = "test-passphrase-123"

// newRepo creates a throwaway git repo (via go-git, no git binary) and returns
// its root.
func newRepo(t *testing.T) (string, *git.Repository) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	return dir, repo
}

// openIn opens an engine rooted at dir (chdir + passphrase env).
func openIn(t *testing.T, dir string) *Engine {
	t.Helper()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv(crypto.PassphraseEnv, pass)
	e, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return e
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// commitAll stages everything and commits, returning nothing (test helper).
func commitAll(t *testing.T, repo *git.Repository, msg string) {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := wt.AddGlob("."); err != nil {
		t.Fatal(err)
	}
	_, err = wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@e.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEndToEnd_InitRegisterSync(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)

	if err := e.Init(nil, pass); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !e.Initialized() {
		t.Fatal("engine should be initialized")
	}

	writeFile(t, filepath.Join(dir, "apps/x/.env.production"), "FOO=bar\nBAZ=qux\n")

	n, err := e.Register("")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if n != 2 {
		t.Fatalf("registered %d keys, want 2", n)
	}

	sp, err := e.DecryptStore()
	if err != nil {
		t.Fatal(err)
	}
	got := sp["apps/x"]["production"][store.Buildtime]
	if got["FOO"] != "bar" || got["BAZ"] != "qux" {
		t.Fatalf("store values wrong: %#v", got)
	}
}

func TestEndToEnd_StoreToLeafRegenerates(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}

	cell := Cell{"apps/y", "_", store.Runtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"TOKEN": "abc"}); err != nil {
		t.Fatal(err)
	}
	leafPath := filepath.Join(dir, "apps/y/.dev.vars")
	if got := readFile(t, leafPath); !strings.Contains(got, "TOKEN=abc") {
		t.Fatalf("leaf not generated: %q", got)
	}

	// Delete the leaf, then StoreToLeaf regenerates it (missing file != deletion).
	if err := os.Remove(leafPath); err != nil {
		t.Fatal(err)
	}
	plan, err := e.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(plan, StoreToLeaf); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, leafPath); !strings.Contains(got, "TOKEN=abc") {
		t.Fatalf("leaf not regenerated: %q", got)
	}
}

func TestEndToEnd_LocalEditCapturedToStore(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/z", "development", store.Buildtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"A": "1"}); err != nil {
		t.Fatal(err)
	}

	// Dev edits the leaf: change A, add B.
	writeFile(t, filepath.Join(dir, "apps/z/.env.development"), "A=2\nB=new\n")

	plan, err := e.Plan()
	if err != nil {
		t.Fatal(err)
	}
	conflicts, err := e.Apply(plan, LeafToStore)
	if err != nil {
		t.Fatal(err)
	}
	if conflicts != 0 {
		t.Fatalf("unexpected conflicts: %d", conflicts)
	}

	sp, _ := e.DecryptStore()
	got := sp["apps/z"]["development"][store.Buildtime]
	if got["A"] != "2" || got["B"] != "new" {
		t.Fatalf("store not updated from leaf: %#v", got)
	}
}

func TestEndToEnd_DeletedKeyPropagates(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/d", "_", store.Buildtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"A": "1", "B": "2"}); err != nil {
		t.Fatal(err)
	}
	// Dev removes B from the leaf.
	writeFile(t, filepath.Join(dir, "apps/d/.env"), "A=1\n")
	plan, err := e.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(plan, LeafToStore); err != nil {
		t.Fatal(err)
	}
	sp, _ := e.DecryptStore()
	got := sp["apps/d"]["_"][store.Buildtime]
	if _, ok := got["B"]; ok {
		t.Fatalf("B should have been deleted from store: %#v", got)
	}
	if got["A"] != "1" {
		t.Fatalf("A should remain: %#v", got)
	}
}

func TestEndToEnd_ConflictBlocksAndResolves(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/c", "_", store.Buildtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"K": "base"}); err != nil {
		t.Fatal(err)
	}

	// Divergence: store advances past base (simulating an incoming pull) while
	// the leaf is hand-edited differently. The base still holds "base".
	sp, _ := e.DecryptStore()
	setCell(sp, cell, map[string]string{"K": "store-val"})
	if err := e.writeStoreFromPT(sp); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "apps/c/.env"), "K=leaf-val\n")

	plan, err := e.Plan()
	if err != nil {
		t.Fatal(err)
	}
	conflicts, err := e.Apply(plan, LeafToStore)
	if err != nil {
		t.Fatal(err)
	}
	if conflicts != 1 {
		t.Fatalf("want 1 conflict, got %d", conflicts)
	}
	// Store must be unchanged because the commit is blocked.
	sp2, _ := e.DecryptStore()
	if sp2["apps/c"]["_"][store.Buildtime]["K"] != "store-val" {
		t.Fatal("store was mutated despite conflict")
	}
	// Leaf must be left intact (no data loss).
	if !strings.Contains(readFile(t, filepath.Join(dir, "apps/c/.env")), "K=leaf-val") {
		t.Fatal("leaf edit was clobbered")
	}

	// Resolve to the leaf value clears the conflict.
	if err := e.ResolveKey(cell, "K", "leaf-val", false); err != nil {
		t.Fatal(err)
	}
	plan, _ = e.Plan()
	conflicts, err = e.Apply(plan, Both)
	if err != nil {
		t.Fatal(err)
	}
	if conflicts != 0 {
		t.Fatalf("conflict not cleared: %d", conflicts)
	}
}

func TestEndToEnd_HooksInstall(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	if err := e.InstallHooks(); err != nil {
		t.Fatal(err)
	}
	hooksDir, err := e.HooksDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hookNames {
		p := filepath.Join(hooksDir, h)
		if got := readFile(t, p); !strings.Contains(got, hookMarker) || !strings.Contains(got, "ward hook "+h) {
			t.Fatalf("hook %s not installed correctly: %q", h, got)
		}
	}
	if err := e.InstallHooks(); err != nil { // idempotent
		t.Fatal(err)
	}
}

func TestEndToEnd_HooksAppendToExisting(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	hooksDir, err := e.HooksDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	preexisting := "#!/bin/sh\necho existing-hook\n"
	writeFile(t, filepath.Join(hooksDir, "pre-commit"), preexisting)
	if err := e.InstallHooks(); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(hooksDir, "pre-commit"))
	if !strings.Contains(got, "echo existing-hook") {
		t.Fatal("pre-existing hook body was lost")
	}
	if !strings.Contains(got, "ward hook pre-commit") {
		t.Fatal("ward invocation not appended")
	}
}

// TestEndToEnd_HooksHonorCoreHooksPath verifies that install and the doctor
// hook check both target git's configured core.hooksPath (as used by husky),
// not the hardcoded .git/hooks.
func TestEndToEnd_HooksHonorCoreHooksPath(t *testing.T) {
	dir, repo := newRepo(t)
	// Point core.hooksPath at a repo-relative .husky directory.
	cfg, err := repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Raw.Section("core").SetOption("hooksPath", ".husky")
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	if err := e.InstallHooks(); err != nil {
		t.Fatal(err)
	}

	// Hooks must land in .husky, NOT .git/hooks.
	for _, h := range hookNames {
		p := filepath.Join(dir, ".husky", h)
		if got := readFile(t, p); !strings.Contains(got, "ward hook "+h) {
			t.Fatalf("hook %s not installed into .husky: %q", h, got)
		}
		if _, err := os.Stat(filepath.Join(e.GitDir, "hooks", h)); err == nil {
			t.Fatalf("hook %s wrongly written to .git/hooks", h)
		}
	}

	// Doctor must report the hooks as installed by looking in .husky.
	for _, ck := range e.Doctor() {
		if strings.HasPrefix(ck.Name, "hook ") && !ck.OK {
			t.Fatalf("doctor reports %s not installed despite core.hooksPath: %s", ck.Name, ck.Info)
		}
	}
}

func TestEndToEnd_PassphraseRoundTripReopen(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/r", "_", store.Runtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"S": "secret"}); err != nil {
		t.Fatal(err)
	}
	e2, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	sp, err := e2.DecryptStore()
	if err != nil {
		t.Fatal(err)
	}
	if sp["apps/r"]["_"][store.Runtime]["S"] != "secret" {
		t.Fatal("value did not survive reopen")
	}
}

func TestEndToEnd_StableCiphertextMinimalDiff(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"apps/s", "_", store.Buildtime}
	if err := e.SetCellPlaintext(cell, map[string]string{"A": "1", "B": "2"}); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, filepath.Join(dir, ".gitward.json"))
	ctBeforeB := e.Store.Targets["apps/s"]["_"].Buildtime["B"].Ciphertext

	// Change only A; B's ciphertext must be byte-identical (stable), keeping the
	// diff minimal.
	if err := e.SetCellPlaintext(cell, map[string]string{"A": "changed", "B": "2"}); err != nil {
		t.Fatal(err)
	}
	ctAfterB := e.Store.Targets["apps/s"]["_"].Buildtime["B"].Ciphertext
	if ctBeforeB != ctAfterB {
		t.Fatal("unchanged value B produced different ciphertext (diff would be noisy)")
	}
	after := readFile(t, filepath.Join(dir, ".gitward.json"))
	if before == after {
		t.Fatal("expected store to change when A changed")
	}
}

// TestEndToEnd_TwoClonesConflictViaPull exercises the real cross-clone flow: a
// value changes upstream and is pulled while a local uncommitted edit exists,
// producing a conflict that post-merge surfaces without data loss.
func TestEndToEnd_TwoClonesConflictViaPull(t *testing.T) {
	// origin (bare)
	originDir := t.TempDir()
	if _, err := git.PlainInit(originDir, true); err != nil {
		t.Fatal(err)
	}

	// clone A
	aDir := t.TempDir()
	repoA, err := git.PlainClone(aDir, false, &git.CloneOptions{URL: originDir})
	if err != nil {
		// empty origin has no refs to clone; init A and add origin remote instead
		repoA, err = git.PlainInit(aDir, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repoA.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{originDir}}); err != nil {
			t.Fatal(err)
		}
	}

	eA := openIn(t, aDir)
	if err := eA.Init(nil, pass); err != nil {
		t.Fatal(err)
	}
	cell := Cell{"app", "_", store.Buildtime}
	if err := eA.SetCellPlaintext(cell, map[string]string{"K": "v1"}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(aDir, ".gitignore"), ".env*\n.dev.vars*\n")
	commitAll(t, repoA, "init")
	if err := repoA.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("push A: %v", err)
	}

	// clone B from origin
	bDir := t.TempDir()
	repoB, err := git.PlainClone(bDir, false, &git.CloneOptions{URL: originDir})
	if err != nil {
		t.Fatalf("clone B: %v", err)
	}
	eB := openIn(t, bDir)
	// B regenerates its leaf from the store (store->leaf).
	planB, _ := eB.Plan()
	if _, err := eB.Apply(planB, StoreToLeaf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, filepath.Join(bDir, "app/.env")), "K=v1") {
		t.Fatal("B did not regenerate leaf from store")
	}

	// A changes K to v2 and pushes.
	if err := os.Chdir(aDir); err != nil {
		t.Fatal(err)
	}
	eA2, _ := Open("")
	if err := eA2.SetCellPlaintext(cell, map[string]string{"K": "v2"}); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repoA, "K=v2")
	if err := repoA.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("push A2: %v", err)
	}

	// B hand-edits K to v3 (uncommitted), then pulls.
	if err := os.Chdir(bDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(bDir, "app/.env"), "K=v3\n")
	wtB, _ := repoB.Worktree()
	if err := wtB.Pull(&git.PullOptions{RemoteName: "origin"}); err != nil && err != git.NoErrAlreadyUpToDate {
		t.Fatalf("pull B: %v", err)
	}

	// post-merge equivalent: StoreToLeaf must NOT clobber the local v3 edit; it
	// is a conflict (store=v2, leaf=v3, base=v1).
	eB2, _ := Open("")
	planB2, err := eB2.Plan()
	if err != nil {
		t.Fatal(err)
	}
	conflicts, err := eB2.Apply(planB2, StoreToLeaf)
	if err != nil {
		t.Fatal(err)
	}
	if conflicts != 1 {
		t.Fatalf("want 1 conflict on pull, got %d", conflicts)
	}
	if !strings.Contains(readFile(t, filepath.Join(bDir, "app/.env")), "K=v3") {
		t.Fatal("local edit v3 was clobbered by pull (data loss)")
	}
}

// TestCheckExamples_EnvAgnosticExampleMatchesAnyEnv reproduces the searchgov
// drift: keys provisioned only under a non-default env (production) must still
// satisfy a suffix-less, env-agnostic .dev.vars.example.
func TestCheckExamples_EnvAgnosticExampleMatchesAnyEnv(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Real secrets live under the production env only.
	writeFile(t, filepath.Join(dir, "apps/searchgov/.dev.vars.production"),
		"CF-Access-Client-Id=id\nCF-Access-Client-Secret=secret\n")
	if _, err := e.Register(""); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// The committed template is env-agnostic (no env suffix).
	writeFile(t, filepath.Join(dir, "apps/searchgov/.dev.vars.example"),
		"CF-Access-Client-Id=XXX\nCF-Access-Client-Secret=XXX\n")

	for _, c := range e.checkExamples() {
		if c.Name == "example parity" && !c.OK {
			t.Fatalf("example parity should pass; got: %s", c.Info)
		}
	}
}

// TestCheckExamples_MissingKeyIsReported ensures the check still fails when an
// example declares a key absent from every env of the store.
func TestCheckExamples_MissingKeyIsReported(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatalf("Init: %v", err)
	}
	writeFile(t, filepath.Join(dir, "apps/searchgov/.dev.vars.production"),
		"CF-Access-Client-Id=id\n")
	if _, err := e.Register(""); err != nil {
		t.Fatalf("Register: %v", err)
	}
	writeFile(t, filepath.Join(dir, "apps/searchgov/.dev.vars.example"),
		"CF-Access-Client-Id=XXX\nUNKNOWN_KEY=XXX\n")

	found := false
	for _, c := range e.checkExamples() {
		if c.Name == "example parity" {
			if c.OK {
				t.Fatalf("expected parity failure for UNKNOWN_KEY")
			}
			if !strings.Contains(c.Info, "UNKNOWN_KEY") {
				t.Fatalf("info should name the missing key; got: %s", c.Info)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no example parity check emitted")
	}
}

// TestCheckExamples_EnvSpecificExampleIsAntipattern locks in the convention:
// an env-pinned example file (.dev.vars.<env>.example) must be flagged, and it
// must not be treated as a real template for parity purposes.
func TestCheckExamples_EnvSpecificExampleIsAntipattern(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatalf("Init: %v", err)
	}
	writeFile(t, filepath.Join(dir, "apps/searchgov/.dev.vars.production"),
		"CF-Access-Client-Id=id\n")
	if _, err := e.Register(""); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// The antipattern: an env-pinned example instead of .dev.vars.example.
	writeFile(t, filepath.Join(dir, "apps/searchgov/.dev.vars.production.example"),
		"CF-Access-Client-Id=XXX\n")

	var parity, convention *Check
	checks := e.checkExamples()
	for i := range checks {
		switch checks[i].Name {
		case "example parity":
			parity = &checks[i]
		case "example convention":
			convention = &checks[i]
		}
	}
	if convention == nil {
		t.Fatal("no example convention check emitted")
	}
	if convention.OK {
		t.Fatalf("env-specific example should fail the convention check")
	}
	if !strings.Contains(convention.Info, ".dev.vars.production.example") {
		t.Fatalf("convention info should name the offending file; got: %s", convention.Info)
	}
	// The antipattern file must not count as a real template — parity passes.
	if parity == nil || !parity.OK {
		t.Fatalf("parity should pass (env-specific example ignored); got: %+v", parity)
	}
}

// TestCheckExamples_EnvAgnosticExamplePassesConvention verifies the sanctioned
// form does not trip the convention check.
func TestCheckExamples_EnvAgnosticExamplePassesConvention(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(nil, pass); err != nil {
		t.Fatalf("Init: %v", err)
	}
	writeFile(t, filepath.Join(dir, "apps/searchgov/.dev.vars.production"),
		"CF-Access-Client-Id=id\n")
	if _, err := e.Register(""); err != nil {
		t.Fatalf("Register: %v", err)
	}
	writeFile(t, filepath.Join(dir, "apps/searchgov/.dev.vars.example"),
		"CF-Access-Client-Id=XXX\n")

	for _, c := range e.checkExamples() {
		if c.Name == "example convention" && !c.OK {
			t.Fatalf("env-agnostic example should pass convention; got: %s", c.Info)
		}
	}
}
