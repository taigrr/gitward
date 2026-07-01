package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"

	"github.com/taigrr/gitward/internal/crypto"
	"github.com/taigrr/gitward/internal/store"
)

// TestRecipients_NamedFormatRoundTrip verifies the recipients map is stored as
// name -> keys, that an age recipient added by name can unwrap the DEK, and
// that removing the name rewraps and drops it.
func TestRecipients_NamedFormat(t *testing.T) {
	dir, _ := newRepo(t)
	e := openIn(t, dir)

	// Generate a native age identity so the test needs no ssh key on disk.
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	recipientLine := id.Recipient().String()

	// Init with a named age recipient plus the passphrase (so we can still
	// decrypt in-process via GITWARD_PASSPHRASE).
	if err := e.Init(map[string][]string{"alice": {recipientLine}}, pass); err != nil {
		t.Fatal(err)
	}

	// Stored shape: recipients.alice == [recipientLine].
	raw, err := os.ReadFile(filepath.Join(dir, ".gitward.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s store.Store
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	got := s.Keys.Recipients["alice"]
	if len(got) != 1 || got[0] != recipientLine {
		t.Fatalf("recipients[alice] = %v, want [%s]", got, recipientLine)
	}
	if s.Keys.DEKAge == "" {
		t.Fatal("expected an age wrap for the age recipient")
	}

	// The age identity alone must recover the DEK (independent of passphrase).
	dek, err := crypto.UnwrapDEKAge(s.Keys.DEKAge, []age.Identity{id})
	if err != nil {
		t.Fatalf("age identity could not unwrap DEK: %v", err)
	}
	if len(dek) == 0 {
		t.Fatal("empty DEK")
	}

	// Add a second person, then remove alice.
	if err := e.AddRecipientKeys("bob", []string{recipientLine}); err != nil {
		t.Fatal(err)
	}
	recips, _ := e.Recipients()
	if _, ok := recips["bob"]; !ok {
		t.Fatal("bob not added")
	}
	removed, err := e.RemoveRecipient("alice")
	if err != nil || !removed {
		t.Fatalf("remove alice: removed=%v err=%v", removed, err)
	}
	recips, _ = e.Recipients()
	if _, ok := recips["alice"]; ok {
		t.Fatal("alice still present after removal")
	}
}
