package crypto

import (
	"fmt"
	"os"

	"github.com/taigrr/gitward/internal/store"
)

// PassphraseEnv is the environment variable CI (and break-glass users) set to
// unwrap the DEK via the shared scrypt passphrase.
const PassphraseEnv = "GITWARD_PASSPHRASE"

// RecoverDEK tries every configured unwrap path in order and returns the first
// DEK it can recover:
//
//  1. shared passphrase, if GITWARD_PASSPHRASE is set (CI / break-glass);
//  2. on-disk ssh identity (age), for devs using ssh keys;
//  3. gpg-agent, for devs using gpg.
//
// The passphrase is tried first because when it is explicitly provided the
// caller is almost always non-interactive (CI), and we must not block on
// pinentry.
func RecoverDEK(keys store.Keys, sshKeyPath string) ([]byte, error) {
	var errs []error

	if pass := os.Getenv(PassphraseEnv); pass != "" && keys.DEKPassphrase != "" {
		dek, err := UnwrapDEKPassphrase(keys.DEKPassphrase, pass)
		if err == nil {
			return dek, nil
		}
		errs = append(errs, fmt.Errorf("passphrase: %w", err))
	}

	if keys.DEKAge != "" {
		if ids, err := LoadSSHIdentities(sshKeyPath); err == nil {
			if dek, err := UnwrapDEKAge(keys.DEKAge, ids); err == nil {
				return dek, nil
			} else {
				errs = append(errs, fmt.Errorf("ssh/age: %w", err))
			}
		} else {
			errs = append(errs, fmt.Errorf("ssh/age: %w", err))
		}
	}

	if keys.DEKPGP != "" && GPGAvailable() {
		dek, err := UnwrapDEKPGP(keys.DEKPGP)
		if err == nil {
			return dek, nil
		}
		errs = append(errs, fmt.Errorf("gpg: %w", err))
	}

	return nil, fmt.Errorf("could not recover data key by any method: %v", errs)
}
