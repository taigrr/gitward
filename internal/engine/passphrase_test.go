package engine

import (
	"testing"

	"filippo.io/age"

	"github.com/taigrr/gitward/internal/crypto"
)

// initWithAgeAndPass initializes the store with one native age recipient plus
// the shared passphrase and returns the engine, the age identity, and the
// original DEK.
func initWithAgeAndPass(t *testing.T, dir string) (*Engine, *age.X25519Identity, []byte) {
	t.Helper()
	e := openIn(t, dir)
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Init(map[string][]string{"alice": {id.Recipient().String()}}, pass); err != nil {
		t.Fatal(err)
	}
	dek := append([]byte(nil), e.DEK()...)
	if len(dek) != crypto.DEKSize {
		t.Fatalf("DEK length = %d, want %d", len(dek), crypto.DEKSize)
	}
	return e, id, dek
}

// TestSetPassphrase_RotatesOnlyPassphraseSlot proves the new passphrase
// recovers the original DEK, the old passphrase no longer does, and the age
// wrap is left byte-identical.
func TestSetPassphrase_RotatesOnlyPassphraseSlot(t *testing.T) {
	dir, _ := newRepo(t)
	e, id, wantDEK := initWithAgeAndPass(t, dir)

	ageBefore := e.Store.Keys.DEKAge
	passBefore := e.Store.Keys.DEKPassphrase
	if ageBefore == "" || passBefore == "" {
		t.Fatal("expected both age and passphrase wraps after init")
	}

	const newPass = "rotated-passphrase-456"
	if err := e.SetPassphrase(newPass); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}

	if e.Store.Keys.DEKAge != ageBefore {
		t.Fatal("age wrap changed; SetPassphrase must leave it byte-identical")
	}
	if e.Store.Keys.DEKPassphrase == passBefore {
		t.Fatal("passphrase wrap unchanged; expected a rotation")
	}

	gotDEK, err := crypto.UnwrapDEKPassphrase(e.Store.Keys.DEKPassphrase, newPass)
	if err != nil {
		t.Fatalf("new passphrase did not unwrap DEK: %v", err)
	}
	if string(gotDEK) != string(wantDEK) {
		t.Fatal("new passphrase recovered a different DEK")
	}

	if _, err := crypto.UnwrapDEKPassphrase(e.Store.Keys.DEKPassphrase, pass); err == nil {
		t.Fatal("old passphrase still unwraps the rotated wrap")
	}

	// The age recipient must still recover the same DEK after rotation.
	ageDEK, err := crypto.UnwrapDEKAge(e.Store.Keys.DEKAge, []age.Identity{id})
	if err != nil {
		t.Fatalf("age identity could not unwrap DEK after rotation: %v", err)
	}
	if string(ageDEK) != string(wantDEK) {
		t.Fatal("age wrap recovers a different DEK after rotation")
	}
}

// TestSetPassphrase_Empty rejects an empty passphrase.
func TestSetPassphrase_Empty(t *testing.T) {
	dir, _ := newRepo(t)
	e, _, _ := initWithAgeAndPass(t, dir)
	if err := e.SetPassphrase(""); err == nil {
		t.Fatal("SetPassphrase(\"\") must fail")
	}
}

// TestClearPassphrase_RemovesWrapButKeepsAge removes the passphrase wrap while
// leaving the age wrap intact and rejects a redundant second removal.
func TestClearPassphrase_RemovesWrapButKeepsAge(t *testing.T) {
	dir, _ := newRepo(t)
	e, id, wantDEK := initWithAgeAndPass(t, dir)
	ageBefore := e.Store.Keys.DEKAge

	removed, err := e.ClearPassphrase()
	if err != nil || !removed {
		t.Fatalf("ClearPassphrase: removed=%v err=%v", removed, err)
	}
	if e.Store.Keys.DEKPassphrase != "" {
		t.Fatal("passphrase wrap still present after ClearPassphrase")
	}
	if e.Store.Keys.DEKAge != ageBefore {
		t.Fatal("age wrap changed during ClearPassphrase")
	}
	gotDEK, err := crypto.UnwrapDEKAge(e.Store.Keys.DEKAge, []age.Identity{id})
	if err != nil {
		t.Fatalf("age identity could not unwrap DEK after clearing passphrase: %v", err)
	}
	if string(gotDEK) != string(wantDEK) {
		t.Fatal("age wrap recovers a different DEK after clearing passphrase")
	}

	removed, err = e.ClearPassphrase()
	if err != nil {
		t.Fatalf("second ClearPassphrase errored: %v", err)
	}
	if removed {
		t.Fatal("second ClearPassphrase reported a removal")
	}
}

// TestClearPassphrase_RefusesOnlyWrap refuses to strip the sole DEK wrap.
func TestClearPassphrase_RefusesOnlyWrap(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)
	if err := e.Init(map[string][]string{}, pass); err != nil {
		t.Fatal(err)
	}
	if e.Store.Keys.DEKAge != "" || e.Store.Keys.DEKPGP != "" {
		t.Fatal("expected passphrase-only store")
	}
	if _, err := e.ClearPassphrase(); err == nil {
		t.Fatal("ClearPassphrase must refuse to remove the only wrap")
	}
	if e.Store.Keys.DEKPassphrase == "" {
		t.Fatal("passphrase wrap was removed despite the refusal")
	}
}
