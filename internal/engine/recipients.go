package engine

import (
	"fmt"
	"os"

	"github.com/taigrr/gitward/internal/crypto"
	"github.com/taigrr/gitward/internal/store"
)

// Init creates a fresh store with a new data key wrapped to the given age
// recipients, gpg recipients, and (optionally) the shared passphrase. It fails
// if the store already has key material.
func (e *Engine) Init(ageRecipients, pgpRecipients []string, passphrase string) error {
	if e.decoded {
		return fmt.Errorf("store already initialized")
	}
	if len(ageRecipients) == 0 && len(pgpRecipients) == 0 && passphrase == "" {
		return fmt.Errorf("need at least one recipient (age, gpg, or passphrase)")
	}
	dek, err := crypto.NewDEK()
	if err != nil {
		return err
	}
	e.SetDEK(dek)
	e.Store.Version = 1
	if e.Store.Targets == nil {
		e.Store.Targets = map[string]store.Target{}
	}
	if err := e.RewrapDEK(ageRecipients, pgpRecipients, passphrase); err != nil {
		return err
	}
	return e.SaveStore()
}

// RewrapDEK re-wraps the current data key to the supplied recipient sets,
// replacing the store's key material. Values are not touched, so this is cheap
// and produces a small diff. An empty passphrase disables the passphrase wrap.
func (e *Engine) RewrapDEK(ageRecipients, pgpRecipients []string, passphrase string) error {
	if !e.decoded {
		return fmt.Errorf("data key not available")
	}
	keys := store.Keys{}
	if len(ageRecipients) > 0 {
		wrapped, err := crypto.WrapDEKAge(e.dek, ageRecipients)
		if err != nil {
			return err
		}
		keys.AgeRecipients = ageRecipients
		keys.DEKAge = wrapped
	}
	if len(pgpRecipients) > 0 {
		wrapped, err := crypto.WrapDEKPGP(e.dek, pgpRecipients)
		if err != nil {
			return err
		}
		keys.PGPFingerprints = pgpRecipients
		keys.DEKPGP = wrapped
	}
	if passphrase == "" {
		// preserve an existing passphrase wrap if the caller passed the sentinel.
		passphrase = os.Getenv(crypto.PassphraseEnv)
	}
	if passphrase != "" {
		wrapped, err := crypto.WrapDEKPassphrase(e.dek, passphrase)
		if err != nil {
			return err
		}
		keys.DEKPassphrase = wrapped
	} else {
		keys.DEKPassphrase = e.Store.Keys.DEKPassphrase
	}
	e.Store.Keys = keys
	return nil
}

// Recipients returns the current age and gpg recipient identifiers.
func (e *Engine) Recipients() (age, pgp []string, passphrase bool) {
	return e.Store.Keys.AgeRecipients, e.Store.Keys.PGPFingerprints, e.Store.Keys.DEKPassphrase != ""
}

// AddAgeRecipient adds an age/ssh recipient and rewraps.
func (e *Engine) AddAgeRecipient(line string) error {
	age, pgp, _ := e.Recipients()
	for _, r := range age {
		if r == line {
			return nil
		}
	}
	age = append(age, line)
	return e.RewrapDEK(age, pgp, "")
}

// AddPGPRecipient adds a gpg fingerprint and rewraps.
func (e *Engine) AddPGPRecipient(fpr string) error {
	age, pgp, _ := e.Recipients()
	for _, r := range pgp {
		if r == fpr {
			return nil
		}
	}
	pgp = append(pgp, fpr)
	return e.RewrapDEK(age, pgp, "")
}

// RemoveRecipient removes an age or gpg recipient by exact match and rewraps.
func (e *Engine) RemoveRecipient(id string) (bool, error) {
	age, pgp, _ := e.Recipients()
	var found bool
	age, found = removeString(age, id)
	if !found {
		pgp, found = removeString(pgp, id)
	}
	if !found {
		return false, nil
	}
	return true, e.RewrapDEK(age, pgp, "")
}

func removeString(xs []string, target string) ([]string, bool) {
	out := xs[:0:0]
	found := false
	for _, x := range xs {
		if x == target {
			found = true
			continue
		}
		out = append(out, x)
	}
	return out, found
}
