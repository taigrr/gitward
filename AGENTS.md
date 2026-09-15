# AGENTS.md

`gitward` (binary `ward`) manages encrypted, git-tracked secrets for a monorepo.
A single encrypted store (`$GIT_ROOT/.gitward.json`) is fanned out to leaf
`.env` / `.dev.vars` files; local edits are captured back on commit via git
hooks. Module: `github.com/taigrr/gitward`.

## Commands

```
go build ./...          # build
go test ./...           # all tests
go test -race ./...     # race detector (CI-equivalent; must stay green)
go vet ./...            # vet
gofmt -l .              # must print nothing
go build -o /tmp/ward ./cmd/ward   # build the CLI binary
```

There is no Makefile or CI config; the commands above are the full toolchain.
Go version is pinned to **1.27.1** in `go.mod`.

## Architecture

Data flows through layered `internal/` packages (leaf depends on store; engine
depends on all):

- `internal/store` — on-disk JSON shape. Store maps **basepath dir → env →
  `{buildtime, runtime}` → key → encrypted value**. Key names are cleartext
  (SOPS style, meaningful diffs); values are individually encrypted.
- `internal/crypto` — envelope encryption: one random DEK (AES-256-GCM per
  value) wrapped 3 ways (age/ssh, gpg, scrypt passphrase); any one unwraps.
  `RecoverDEK` tries passphrase → ssh → gpg in that order. Recipients are
  stored grouped by name in `Keys.Recipients` (`map[name][]keys`); each key is
  classified at wrap time by `isAgeKey` (ssh-ed25519/ssh-rsa/age1 prefix → age
  wrap, otherwise → gpg fingerprint). One person can hold both an ssh key and a
  gpg fingerprint under one name.
- `internal/merge` — the 3-way merge state machine (store S / leaf L / base B).
  Pure, no I/O, heavily unit-tested. This is the correctness core.
- `internal/leaf` — dotenv parse/serialize + filename convention.
- `internal/gitutil` — repo discovery by walking for `.git` (no config parse,
  so `extensions.worktreeConfig` repos and linked worktrees work); ignore
  matching via go-git's matcher off the raw filesystem; staging shells out to
  `git add` (only ever runs inside the pre-commit hook, where git is present).
- `internal/engine` — ties everything together (`Open`, `Plan`, `Apply`,
  `Init`, `Register`, hooks, doctor). The CLI is a thin shell over this.
- `internal/cli` + `cmd/ward` — cobra commands wrapped by charmbracelet `fang`.

## Non-obvious gotchas

- **Deterministic ciphertext is intentional, not a bug.** `EncryptValueStable`
  derives the GCM nonce from `(DEK, key, plaintext)` so unchanged values
  re-encrypt to byte-identical output, keeping git diffs minimal. Do NOT switch
  to random nonces — a test enforces this.
- **The variable name is bound as AEAD additional data.** Ciphertext cannot be
  moved between keys; decrypting under the wrong name fails by design.
- **The base snapshot (`.git/gitward/base.json`) is the merge-base linchpin.**
  It holds plaintext (same secrets as the `.env` files), lives inside `.git` so
  it is never committed and needs no gitignore entry. Without it the 3-way
  merge degrades to lossy store-wins/leaf-wins. Advance it only for
  non-conflicting cells.
- **A missing leaf file is NOT a deletion.** In `engine.Plan`/`Apply` an absent
  leaf is treated as "no local change" (leaf == base) so fresh clones
  regenerate and pulls never clobber uncommitted edits. A key *removed from an
  existing* file does propagate as a deletion.
- **age cannot use ssh-agent** — only signs, can't derive a decryption key. The
  ssh path (`crypto/identity.go`) requires the private key file on disk
  (`~/.ssh/id_ed25519`, then `id_rsa`). Agent-backed decryption is only via the
  gpg path.
- **gpg is deliberately shelled out** (`crypto/gpg.go`) to preserve gpg-agent /
  pinentry / smartcard support; a pure-Go OpenPGP lib would lose those. This is
  the *only* external binary dependency.
- **Hooks honor `core.hooksPath`.** `InstallHooks` and `Doctor` resolve the
  hooks dir via `gitutil.HooksDir` (`git rev-parse --git-path hooks`), so husky
  / custom `core.hooksPath` setups install and verify in the right place — not
  a hardcoded `.git/hooks`. Install is idempotent (skips if `hookMarker`
  present) and appends to a pre-existing hook body rather than clobbering it.
- **Hooks must never block git except pre-commit on a true conflict.**
  `runHook` (`cli/doctor.go`) swallows most errors with a warning and exits 0;
  `post-checkout`/`post-merge` = store→leaf, `pre-commit` = leaf→store + stage
  the store. `Apply` returns a conflict count and refuses to mutate the store
  when `dir != StoreToLeaf` and conflicts exist.
- **`_` is the sentinel env** (`store.DefaultEnv`) for suffix-less files
  (`.env`, `.dev.vars`). Filenames are convention-derived in `leaf.FileName`:
  buildtime → `.env[.<env>]`, runtime → `.dev.vars[.<env>]`.
- Only `*.example` dotfiles are ever committed; everything else is gitignored.
- **Ignored variables are invisible to the merge but preserved on disk.**
  A ward-ignored variable is stored as an `EncValue{Ignore: true}` marker
  *colocated with where its value would live* — in the cell's `TierMap`, with no
  `enc` — so ignoring is per-cell `(path, env, tier)`. The engine strips ignored
  keys from all three merge inputs (store, leaf, base) in `Plan`/`Apply`/
  `Register`, so they are never captured, written, or reported as drift. On
  store→leaf regeneration `writeLeaf` re-emits any ignored keys (from the on-disk
  leaf, else a seed value) in a trailing block after `leaf.IgnoredHeader` —
  emitted *only* when at least one ignored variable is present. Markers carry no
  plaintext, so `DecryptStore` skips them and `writeStoreFromPT` re-injects them
  via `reinjectIgnores` (otherwise the rebuild-from-plaintext would drop them).
  `engine.Ignore` evicts an already-stored key from store+base and preserves its
  last-known value into the leaf even when the leaf file is absent.

## Testing patterns

- Integration tests (`engine/engine_test.go`) build real repos in-process with
  **go-git** (`git.PlainInit`/`PlainClone`), not by shelling to `git`.
- Test helpers `chdir` into a temp repo and set `GITWARD_PASSPHRASE`
  (`crypto.PassphraseEnv`) via `t.Setenv`, then unlock the store with the
  passphrase recipient — this is also how CI decrypts.
- `merge_test.go` covers the full state-machine table; keep it exhaustive when
  changing merge rules.
- Prefer adding cases to the existing table-driven tests over new ad-hoc ones.

## Not yet implemented (future work noted in the design doc)

`wrangler secret put` push hook, secret-rotation helpers, and a bubbletea TUI
(everything is CLI today).
