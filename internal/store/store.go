// Package store defines the on-disk shape of the gitward secret store
// ($GIT_ROOT/.gitward.json) and the plaintext base snapshot
// (.git/gitward/base.json).
//
// The store is a single JSON file. Keys (variable names) are cleartext so git
// diffs stay meaningful; values are individually encrypted (SOPS-style) and are
// only re-encrypted when their plaintext changes, keeping diffs stable.
package store

import "sort"

// Tier distinguishes build-time variables (baked into a client bundle, e.g.
// NEXT_PUBLIC_*) from run-time variables (server/worker secrets pushed to
// `wrangler secret put`).
type Tier string

const (
	// Buildtime values are written to a .env[.<env>] file the build reads.
	Buildtime Tier = "buildtime"
	// Runtime values are written to a .dev.vars[.<env>] file and are the set
	// pushed to the Workers runtime as secrets.
	Runtime Tier = "runtime"
)

// DefaultEnv is the sentinel env whose generated files carry no suffix
// (.env and .dev.vars rather than .env.<env>).
const DefaultEnv = "_"

// EncValue is a single encrypted variable value. Ciphertext is the armored
// AES-256-GCM payload; it stays byte-identical across writes when the plaintext
// is unchanged so the JSON store produces minimal git diffs.
// EncValue is a single variable entry. For a managed variable Ciphertext is the
// armored AES-256-GCM payload; it stays byte-identical across writes when the
// plaintext is unchanged so the JSON store produces minimal git diffs. For a
// ward-ignored variable Ciphertext is empty and Ignore is true: the variable is
// recorded so it is never managed, but no value is stored.
type EncValue struct {
	Ciphertext string `json:"enc,omitempty"`
	Ignore     bool   `json:"ignore,omitempty"`
}

// TierMap maps a variable name to its encrypted value for one tier.
type TierMap map[string]EncValue

// EnvBlock holds the buildtime and runtime variable sets for a single
// environment of a single target.
type EnvBlock struct {
	Buildtime TierMap `json:"buildtime,omitempty"`
	Runtime   TierMap `json:"runtime,omitempty"`
}

// Target is one basepath (a directory, relative to the git root) that receives
// generated env files. Its key in Store.Targets is that directory.
type Target map[string]EnvBlock // env name -> block

// Keys holds the wrapped data-encryption-key (DEK) material. Any one wrap can
// recover the DEK, so a dev may use ssh (age), gpg, or the shared passphrase.
type Keys struct {
	// Recipients maps a human-readable name (typically a GitHub username) to
	// that person's public keys. Each key is either an ssh/age recipient line
	// ("ssh-ed25519 ...", "ssh-rsa ...", "age1...") or a gpg fingerprint/key id;
	// the kind is classified at wrap time. Grouping by name keeps diffs and
	// membership auditing meaningful (who has access, not just which keys).
	Recipients map[string][]string `json:"recipients,omitempty"`
	// DEKAge is the age-armored DEK (recoverable by any age/ssh recipient).
	DEKAge string `json:"dek_age,omitempty"`
	// DEKPGP is the gpg-encrypted DEK (recoverable via gpg-agent).
	DEKPGP string `json:"dek_pgp,omitempty"`
	// DEKPassphrase is the scrypt-passphrase-wrapped DEK (CI / break-glass).
	DEKPassphrase string `json:"dek_passphrase,omitempty"`
}

// Store is the root document persisted to $GIT_ROOT/.gitward.json.
type Store struct {
	Version int               `json:"version"`
	Keys    Keys              `json:"keys"`
	Targets map[string]Target `json:"targets"`
}

// tierMap returns the TierMap for a (path, env, tier) cell, or nil if absent.
func (s *Store) tierMap(path, env string, tier Tier) TierMap {
	blk, ok := s.Targets[path][env]
	if !ok {
		return nil
	}
	if tier == Runtime {
		return blk.Runtime
	}
	return blk.Buildtime
}

// Ignored reports whether key is marked ward-ignored in the (path, env, tier)
// cell.
func (s *Store) Ignored(path, env string, tier Tier, key string) bool {
	return s.tierMap(path, env, tier)[key].Ignore
}

// IgnoredNames returns the sorted names marked ward-ignored in the (path, env,
// tier) cell.
func (s *Store) IgnoredNames(path, env string, tier Tier) []string {
	var out []string
	for k, v := range s.tierMap(path, env, tier) {
		if v.Ignore {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Plaintext is the fully decrypted view of a target/env/tier, used by the merge
// engine and the base snapshot. It mirrors the store shape but with cleartext
// values.
type Plaintext map[string]map[string]map[Tier]map[string]string // path -> env -> tier -> key -> value

// Base is the plaintext snapshot from the last successful sync, stored at
// .git/gitward/base.json. It is the merge base that makes drift resolution
// lossless (see internal/merge).
type Base struct {
	Version int       `json:"version"`
	Data    Plaintext `json:"data"`
}
