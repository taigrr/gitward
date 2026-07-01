package engine

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/taigrr/gitward/internal/crypto"
	"github.com/taigrr/gitward/internal/store"
)

// Init creates a fresh store with a new data key wrapped to the given
// recipients (name -> keys) and, optionally, the shared passphrase. It fails
// if the store already has key material.
func (e *Engine) Init(recipients map[string][]string, passphrase string) error {
	if e.decoded {
		return fmt.Errorf("store already initialized")
	}
	if countKeys(recipients) == 0 && passphrase == "" {
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
	if err := e.RewrapDEK(recipients, passphrase); err != nil {
		return err
	}
	return e.SaveStore()
}

// RewrapDEK re-wraps the current data key to the supplied recipients (name ->
// keys), replacing the store's key material. Keys are classified into age
// (ssh/age lines) and gpg (fingerprints) at wrap time. Values are not touched,
// so this is cheap and produces a small diff. An empty passphrase preserves an
// existing passphrase wrap (or picks up GITWARD_PASSPHRASE if set).
func (e *Engine) RewrapDEK(recipients map[string][]string, passphrase string) error {
	if !e.decoded {
		return fmt.Errorf("data key not available")
	}
	ageLines, gpgFprs := splitRecipients(recipients)

	keys := store.Keys{Recipients: normalizeRecipients(recipients)}
	if len(ageLines) > 0 {
		wrapped, err := crypto.WrapDEKAge(e.dek, ageLines)
		if err != nil {
			return err
		}
		keys.DEKAge = wrapped
	}
	if len(gpgFprs) > 0 {
		wrapped, err := crypto.WrapDEKPGP(e.dek, gpgFprs)
		if err != nil {
			return err
		}
		keys.DEKPGP = wrapped
	}
	if passphrase == "" {
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

// Recipients returns a copy of the current name -> keys map and whether the
// shared passphrase wrap is present.
func (e *Engine) Recipients() (map[string][]string, bool) {
	return normalizeRecipients(e.Store.Keys.Recipients), e.Store.Keys.DEKPassphrase != ""
}

// AddRecipientKeys adds one or more keys under a recipient name (creating the
// entry if needed), de-duplicates, and rewraps. Keys may be ssh/age lines or
// gpg fingerprints.
func (e *Engine) AddRecipientKeys(name string, newKeys []string) error {
	if name == "" {
		return fmt.Errorf("recipient name is required")
	}
	recips, _ := e.Recipients()
	if recips == nil {
		recips = map[string][]string{}
	}
	existing := recips[name]
	for _, k := range newKeys {
		k = strings.TrimSpace(k)
		if k == "" || slices.Contains(existing, k) {
			continue
		}
		existing = append(existing, k)
	}
	recips[name] = existing
	return e.RewrapDEK(recips, "")
}

// RemoveRecipient removes an entire named recipient and rewraps. Returns false
// if the name was not present.
func (e *Engine) RemoveRecipient(name string) (bool, error) {
	recips, _ := e.Recipients()
	if _, ok := recips[name]; !ok {
		return false, nil
	}
	delete(recips, name)
	return true, e.RewrapDEK(recips, "")
}

// isAgeKey reports whether a key line is an age/ssh recipient (as opposed to a
// gpg fingerprint/key id).
func isAgeKey(k string) bool {
	return strings.HasPrefix(k, "ssh-ed25519 ") ||
		strings.HasPrefix(k, "ssh-rsa ") ||
		strings.HasPrefix(k, "age1")
}

// splitRecipients flattens the name -> keys map into deduplicated, sorted age
// recipient lines and gpg fingerprints.
func splitRecipients(recipients map[string][]string) (age, gpg []string) {
	ageSet := map[string]struct{}{}
	gpgSet := map[string]struct{}{}
	for _, keys := range recipients {
		for _, k := range keys {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if isAgeKey(k) {
				ageSet[k] = struct{}{}
			} else {
				gpgSet[k] = struct{}{}
			}
		}
	}
	return sortedKeys(ageSet), sortedKeys(gpgSet)
}

// normalizeRecipients returns a deep copy with trimmed, de-duplicated,
// sorted keys and empty entries dropped.
func normalizeRecipients(recipients map[string][]string) map[string][]string {
	if len(recipients) == 0 {
		return nil
	}
	out := map[string][]string{}
	for name, keys := range recipients {
		seen := map[string]struct{}{}
		var cleaned []string
		for _, k := range keys {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			cleaned = append(cleaned, k)
		}
		if len(cleaned) > 0 {
			sort.Strings(cleaned)
			out[name] = cleaned
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func countKeys(recipients map[string][]string) int {
	n := 0
	for _, keys := range recipients {
		for _, k := range keys {
			if strings.TrimSpace(k) != "" {
				n++
			}
		}
	}
	return n
}

func sortedKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
