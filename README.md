<picture>
  <img alt="gitward" src="https://github.com/taigrr/gitward/raw/master/docs/mascot.jpg" width="400">
</picture>

<p>
  <a href="https://github.com/taigrr/gitward/releases/latest">
    <img alt="Latest release" src="https://img.shields.io/github/v/release/taigrr/gitward?style=for-the-badge&logo=starship&color=FF6AD5&logoColor=D9E0EE&labelColor=302D41&sort=semver&include_prerelease">
  </a>
  <a href="https://github.com/taigrr/gitward/pulse">
    <img alt="Last commit" src="https://img.shields.io/github/last-commit/taigrr/gitward?style=for-the-badge&logo=starship&color=8bd5ca&labelColor=302D41&logoColor=D9E0EE">
  </a>
  <a href="https://github.com/taigrr/gitward/blob/master/LICENSE">
    <img alt="License" src="https://img.shields.io/github/license/taigrr/gitward?style=for-the-badge&logo=starship&color=ee999f&labelColor=302D41&logoColor=D9E0EE">
  </a>
  <a href="https://github.com/taigrr/gitward/stargazers">
    <img alt="Stars" src="https://img.shields.io/github/stars/taigrr/gitward?style=for-the-badge&logo=starship&color=c69ff5&labelColor=302D41&logoColor=D9E0EE">
  </a>
</p>

**Git + Ward = gitward** — Painless, git-tracked, encrypted secrets for your monorepo.

## The Problem

Secrets in a monorepo turn into a mess:

- Some live in `wrangler secret put`, only in the cloud
- Some live in `.env`, `.env.production`, `.env.preview`, gitignored and passed around Slack
- Some live in `.dev.vars`, copied by hand between machines

There is no single source of truth, no safe way to keep them in the repo, and
onboarding a new dev means hunting down every scattered value.

## The Solution

**gitward** keeps one encrypted store in your repo and fans it out to leaf files:

```sh
# One encrypted store, committed to git — a leaked repo leaks no plaintext
$GIT_ROOT/.gitward.json

# ward generates .env / .dev.vars from it, and captures your edits back on commit
ward install   # hooks do the rest
```

Secrets stay in git but encrypted, diffs stay meaningful, and edits flow both
ways automatically.

## ✨ Features

- 🔒 **In git, but safe** — Secrets are committed encrypted; a leaked repo leaks no plaintext
- 📝 **Plaintext JSON, minimal diffs** — Key names stay cleartext (SOPS style); values encrypt to stable ciphertext
- 🔁 **Lossless three-way merge** — Store, leaf, and base reconcile like git; edits are never silently clobbered
- 🪝 **Git-hook driven** — `post-checkout`/`post-merge` fan out, `pre-commit` captures back, never blocks except on a real conflict
- 🔑 **Multiple unlock paths** — age/ssh keys, gpg-agent, or a shared passphrase — any one recovers the data key
- 🤖 **CI-friendly** — A single `GITWARD_PASSPHRASE` unlocks everything in CI
- 🧰 **No git binary required** — Repo operations use go-git; only `gpg` is shelled out
- 🩺 **Batteries included** — `doctor` checks hooks, recipients, gitignore coverage, and `.example` parity

## 📦 Installation

```sh
go install github.com/taigrr/gitward/cmd/ward@latest
```

Then wire the hooks into your repo (idempotent — safe to run from a `bun`/`npm`
postinstall):

```sh
ward install
```

## ⚡ Requirements

- Go >= **1.26** (to build)
- Optional: `gpg` with gpg-agent (for the gpg unlock path)
- Optional: an ssh key at `~/.ssh/id_ed25519` or `~/.ssh/id_rsa` (for the ssh unlock path)

## 🚀 Quick Start

```sh
# Create the store, wrapping the data key to your team's GitHub ssh keys
# plus a shared passphrase for CI (read from GITWARD_PASSPHRASE)
ward init --github alice,bob,carol --passphrase

# Install git hooks
ward install

# Capture existing .env / .dev.vars files into the store
ward register

# Edit a secret; the change is captured back on your next commit
ward edit apps/web production runtime
```

## 🗂️ Model

```
$GIT_ROOT/.gitward.json      encrypted store (committed)
.git/gitward/base.json       plaintext merge base (never committed, hidden)
<basepath>/.env[.<env>]      generated buildtime leaf   (gitignored)
<basepath>/.dev.vars[.<env>] generated runtime  leaf   (gitignored)
```

The store maps a **basepath (directory)** → **env** → `{buildtime, runtime}`.
Filenames are derived by convention; the sentinel env `_` produces suffix-less
files (`.env`, `.dev.vars`). Only `*.example` dotfiles are ever committed.

### The three-way merge (why edits are never lost)

Every reconciliation compares three sources per key:

- **S** — value in the store (last committed truth)
- **L** — value in the leaf file on disk
- **B** — value in the base snapshot (last successful sync)

| S vs B  | L vs B        | Result                       |
| ------- | ------------- | ---------------------------- |
| same    | same          | no-op                        |
| changed | same          | take store (incoming update) |
| same    | changed       | take leaf (local edit)       |
| changed | changed, `==` | converged, advance base      |
| changed | changed, `!=` | **conflict** (block/prompt)  |

`post-checkout` / `post-merge` apply store→leaf; `pre-commit` applies
leaf→store and **blocks only on a true conflict**. A missing leaf file is
treated as "no local change" (it is disposable), so a fresh clone regenerates
cleanly and a pull never clobbers an uncommitted local edit.

## 📖 Commands

| Command                                        | Description                                       |
| ---------------------------------------------- | ------------------------------------------------- |
| `ward init [flags]`                            | Create the store and data key                     |
| `ward install`                                 | Install git hooks (idempotent)                    |
| `ward register [path]`                         | Capture new keys from leaf files into the store   |
| `ward edit <path> <env> <buildtime\|runtime>`  | `$EDITOR` a cell, capture back                    |
| `ward sync`                                    | Reconcile both directions                         |
| `ward status`                                  | Per-cell drift summary                            |
| `ward diff [path]`                             | Per-key merge decisions                           |
| `ward resolve [path]`                          | Interactively resolve conflicts                   |
| `ward list`                                    | Targets, envs, tiers, key names (never values)    |
| `ward add-recipient <ssh\|gpg\|github>`        | Add a recipient and rewrap the data key           |
| `ward rm-recipient <recipient>`                | Remove a recipient and rewrap the data key        |
| `ward doctor`                                  | Check store, hooks, recipients, gitignore parity  |

### `ward init` flags

| Flag                     | Description                                             |
| ------------------------ | ------------------------------------------------------ |
| `--github USER,...`      | Fetch each user's GitHub ssh keys as recipients        |
| `--ssh-recipient LINE`   | Add an explicit age/ssh recipient line                 |
| `--gpg FPR`              | Add a gpg recipient fingerprint                        |
| `--passphrase`           | Also wrap the data key with `GITWARD_PASSPHRASE`       |

## 🤖 CI

Set `GITWARD_PASSPHRASE` in the CI environment. `ward` recovers the data key
from the passphrase wrap without any ssh key or gpg-agent, then `ward sync`
materializes the leaf files.

## 🔐 How Unlocking Works

The store is protected by envelope encryption: one random data-encryption key
(DEK) encrypts every value, and the DEK itself is wrapped independently for
each recipient class. **Any one wrap recovers the DEK**, so different devs can
use whatever they are comfortable with:

- **age / ssh** — reads your on-disk private key (`~/.ssh/id_ed25519`, then `id_rsa`)
- **gpg** — via gpg-agent (pinentry, cached passphrases, and smartcards all work)
- **passphrase** — a shared scrypt passphrase for CI and break-glass recovery

## 🩺 Security Notes

- Removing a recipient rewraps the data key but does **not** rotate secret
  values; prior ciphertext remains in git history. Rotate the actual values at
  the provider after removing someone.
- The base snapshot in `.git/gitward/base.json` holds plaintext — the same
  secrets already in your `.env` files — and is never committed.

## 🗺️ Roadmap

- `wrangler secret put` push hook for runtime secrets
- Secret rotation helpers
- A [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI

## 📄 License

[0BSD](LICENSE) © [Tai Groot](https://github.com/taigrr)
