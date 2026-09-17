---
name: "gitward"
description: "Manage encrypted, git-tracked secrets with gitward (the `ward` CLI). Use whenever you create or change a .env, .env.*, .dev.vars, or .dev.vars.* file, add/remove/rotate a secret key, resolve a ward conflict, onboard/offboard a recipient, or a user asks how to work with ward — and a .gitward.json store exists at the repo root. Covers both the interactive commands humans use and the non-interactive/JSON commands agents must use."
---

# gitward (`ward`) — encrypted, git-tracked secrets

One encrypted store at `$GIT_ROOT/.gitward.json` (committed) is fanned out to
per-directory **leaf files** (`.env`, `.env.<env>`, `.dev.vars`,
`.dev.vars.<env>`; all gitignored). Local edits to leaf files are captured back
into the store on commit via git hooks. Key names are cleartext; values are
encrypted. A leaked repo leaks no plaintext.

## Two ways to drive it

`ward` has an **interactive surface for humans** and a **non-interactive surface
for agents and scripts**. They operate on the same data and are interchangeable;
pick by who is at the keyboard.

| Task | Human (terminal) | Agent / script |
| --- | --- | --- |
| Read a value | `ward get <cell> KEY` | `ward get <cell> KEY --json` |
| Change values | `ward edit <cell>` (opens `$EDITOR`) | `ward set <cell> KEY=VALUE...` or `printf '%s' "$V" \| ward set <cell> KEY --stdin` |
| Remove a key | `ward edit <cell>` and delete the line | `ward unset <cell> KEY...` |
| Check drift | `ward status` | `ward status --exit-code --json` |
| Preview a sync | `ward diff` | `ward sync --dry-run --json` |
| Reconcile | `ward sync` | `ward sync --json` (exit 2 on conflict) |
| Resolve a conflict | `ward resolve` (prompts per key) | `ward resolve --take store\|leaf` or `--key K --value V` / `--key K --delete` |
| Import new keys | `ward register [path]` | `ward register --dry-run --json`, then `ward register --json` |
| Inspect | `ward list`, `ward recipients`, `ward doctor` | same, with `--json` |

When a **user asks how to do something**, answer with the human column. When
**you are doing it**, use the agent column: never invoke `ward edit` or a bare
`ward resolve` yourself — `edit` opens an editor you cannot drive, and
`resolve` without flags errors when stdin is not a terminal.

## Addressing a cell

A *cell* is one `(path, env, tier)` location. Every command that takes a cell
accepts either form:

```
ward get apps/web/.env.production KEY          # leaf-file form (preferred)
ward get apps/web production buildtime KEY     # explicit triple
```

- `buildtime` → `.env[.<env>]` (read by the build, e.g. `NEXT_PUBLIC_*`)
- `runtime` → `.dev.vars[.<env>]` (server secrets; wrangler)
- env `_` is the sentinel for suffix-less files (`.env`, `.dev.vars`)
- Paths are relative to your current directory; commands work from any subdir.

## Agent loop

```sh
ward status --exit-code --json      # 0 clean, 1 pending changes, 2 conflict
ward sync --dry-run --json          # inspect {changes:[{file,key,op}]}
ward sync --json                    # apply; exit 2 and no store mutation on conflict
ward resolve --take leaf --json     # or --key K --value V / --key K --delete
printf '%s' "$SECRET" | ward set apps/api/.dev.vars API_KEY --stdin --json
ward get apps/api/.dev.vars API_KEY
git add .gitward.json               # the store must travel with the commit
```

- `--json` is global: data on stdout, errors on stderr as
  `{"error": "...", "kind": "error"|"conflict"}`. Nothing is styled.
- `set`/`unset` refuse to write when the cell has unsynced changes (they
  replace the leaf file). Run `ward sync` first, or pass `--force` if you are
  sure the local edit should be discarded.
- `op` values in JSON: `incoming` (store→leaf), `local` (leaf→store),
  `delete`, `conflict`. Human `diff` uses `<-`, `->`, `--`, `!!`.
- Values are printed **only** by `get`. `list`, `status`, `diff`, `sync`
  never emit plaintext, so they are safe in logs.

## Adding a new secret (agent)

1. Add the key with a placeholder to the committed `*.example` file next to the
   leaf (e.g. `apps/api/.dev.vars.example`) so example parity holds.
2. Pick the tier (build-time client config → `buildtime`; server secret →
   `runtime`).
3. `printf '%s' "$VALUE" | ward set apps/api/.dev.vars KEY --stdin --json`
   This writes store, leaf file, and merge base in one step; no `register` or
   `sync` needed.
4. `ward status --exit-code && ward doctor`, then stage `.gitward.json`.

If a human has already put the key in the leaf file by hand, use
`ward register apps/api` instead of `set` to import it.

## Conflicts

A conflict means the store and the leaf both changed the same key since the
last sync. `pre-commit` blocks on it; nothing else does. Inspect with
`ward diff <path>` (keys only) or `ward get <cell> KEY` (store side) and read
the leaf file (local side), then resolve:

```sh
ward resolve --key KEY --take store     # keep incoming
ward resolve --key KEY --take leaf      # keep local edit
ward resolve --key KEY --value NEW      # pick a third value
ward resolve --take leaf                # everything, one side
```

## Unlocking

Any one of these recovers the data key: an ssh/age private key **on disk**
(`~/.ssh/id_ed25519`, then `id_rsa`; ssh-agent alone cannot decrypt; override
with `--ssh-key`), a gpg key via gpg-agent, or `GITWARD_PASSPHRASE` in the
environment (how CI unlocks). `ward recipients` lists who can decrypt.

## Recipients

```sh
ward add-recipient alice --github                 # fetch alice's GitHub ssh keys
ward add-recipient alice "ssh-ed25519 AAAA..."    # explicit key
ward add-recipient alice DEADBEEF...              # gpg fingerprint
ward rm-recipient alice
```

`rm-recipient` rewraps the data key so they cannot decrypt future commits, but
prior ciphertext remains in git history — rotate any value they could read.

## Gotchas

- Only `.gitward.json` and `*.example` files are committed; never commit a real
  leaf file.
- Deterministic ciphertext is intentional (stable git diffs). Unchanged values
  re-encrypt byte-identically.
- A **missing** leaf file is not a deletion (fresh clones regenerate); a key
  removed from an **existing** leaf file does propagate as a deletion.
- Ward-ignored variables (`ward ignore <cell> KEY`) live in the leaf under a
  `# ward-ignored variables below` header and are never captured or reported.
- Hooks honor `core.hooksPath`; `ward doctor` verifies they are installed.
- Always stage `.gitward.json` after `set`, `unset`, `register`, `resolve`, or
  recipient changes.
