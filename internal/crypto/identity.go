package crypto

import (
	"fmt"
	"os"
	"path/filepath"

	"filippo.io/age"
	"filippo.io/age/agessh"
)

// LoadSSHIdentities discovers the caller's ssh private key for age decryption,
// mirroring ssh-mcp's convention: an explicit key path first, then
// ~/.ssh/id_ed25519, then ~/.ssh/id_rsa.
//
// NOTE: unlike ssh authentication, age decryption cannot use ssh-agent. The
// agent only performs signatures; age derives an X25519/RSA *decryption* key
// from the private key material, which the agent will not expose. The on-disk
// private key is therefore required for the ssh path (agent-backed decryption
// is available via the separate gpg path). This trade-off was accepted in the
// design ("on-disk is ok").
func LoadSSHIdentities(keyPath string) ([]age.Identity, error) {
	paths := candidateKeyPaths(keyPath)
	var ids []age.Identity
	var lastErr error
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		id, err := agessh.ParseIdentity(b)
		if err != nil {
			lastErr = fmt.Errorf("parsing ssh key %s: %w", p, err)
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("no usable ssh identity found (tried %v)", paths)
	}
	return ids, nil
}

func candidateKeyPaths(keyPath string) []string {
	if keyPath != "" {
		return []string{keyPath}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
	}
}
