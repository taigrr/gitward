// Package engine ties the store, crypto, leaf, and merge packages together into
// the operations the CLI and git hooks invoke: loading and decrypting the
// store, reading leaf files, running the 3-way merge against the base snapshot,
// and applying the results in either direction.
package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/taigrr/gitward/internal/crypto"
	"github.com/taigrr/gitward/internal/gitutil"
	"github.com/taigrr/gitward/internal/leaf"
	"github.com/taigrr/gitward/internal/merge"
	"github.com/taigrr/gitward/internal/store"
)

// Engine holds resolved paths and the decrypted data key for a repo.
type Engine struct {
	Root    string
	GitDir  string
	Store   *store.Store
	dek     []byte
	sshKey  string
	decoded bool
}

// Open locates the repo, loads the store (creating an empty one if absent), and
// recovers the data key. sshKeyPath may be empty to use default ssh key
// discovery.
func Open(sshKeyPath string) (*Engine, error) {
	root, err := gitutil.Root("")
	if err != nil {
		return nil, err
	}
	gitDir, err := gitutil.GitDir(root)
	if err != nil {
		return nil, err
	}
	s, err := loadStore(gitutil.StorePath(root))
	if err != nil {
		return nil, err
	}
	e := &Engine{Root: root, GitDir: gitDir, Store: s, sshKey: sshKeyPath}
	// Recover DEK only if the store is initialized (has key material).
	if s.Keys.DEKAge != "" || s.Keys.DEKPGP != "" || s.Keys.DEKPassphrase != "" {
		dek, err := crypto.RecoverDEK(s.Keys, sshKeyPath)
		if err != nil {
			return nil, err
		}
		e.dek = dek
		e.decoded = true
	}
	return e, nil
}

// Initialized reports whether the store has any key material.
func (e *Engine) Initialized() bool { return e.decoded }

// DEK exposes the data key for operations that need to (re)wrap it.
func (e *Engine) DEK() []byte { return e.dek }

// SetDEK installs a freshly generated data key (used by init).
func (e *Engine) SetDEK(dek []byte) {
	e.dek = dek
	e.decoded = true
}

func loadStore(path string) (*store.Store, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &store.Store{Version: 1, Targets: map[string]store.Target{}}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var s store.Store
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if s.Targets == nil {
		s.Targets = map[string]store.Target{}
	}
	return &s, nil
}

// SaveStore writes the store back to disk with stable, indented JSON.
func (e *Engine) SaveStore() error {
	b, err := json.MarshalIndent(e.Store, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(gitutil.StorePath(e.Root), b, 0o644)
}

// DecryptStore returns the fully decrypted plaintext view of the store.
func (e *Engine) DecryptStore() (store.Plaintext, error) {
	if !e.decoded {
		return nil, fmt.Errorf("store is not initialized")
	}
	out := store.Plaintext{}
	for path, tgt := range e.Store.Targets {
		out[path] = map[string]map[store.Tier]map[string]string{}
		for env, blk := range tgt {
			out[path][env] = map[store.Tier]map[string]string{}
			bt, err := e.decTier(blk.Buildtime)
			if err != nil {
				return nil, err
			}
			rt, err := e.decTier(blk.Runtime)
			if err != nil {
				return nil, err
			}
			out[path][env][store.Buildtime] = bt
			out[path][env][store.Runtime] = rt
		}
	}
	return out, nil
}

func (e *Engine) decTier(m store.TierMap) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range m {
		pt, err := crypto.DecryptValue(e.dek, k, v.Ciphertext)
		if err != nil {
			return nil, err
		}
		out[k] = pt
	}
	return out, nil
}

// encTier encrypts a plaintext map into a TierMap. EncryptValueStable is a pure
// function of (DEK, key, plaintext), so unchanged values re-encrypt to
// byte-identical ciphertext and the store's git diff stays minimal.
func (e *Engine) encTier(vals map[string]string) (store.TierMap, error) {
	out := store.TierMap{}
	for k, v := range vals {
		ct, err := crypto.EncryptValueStable(e.dek, k, v)
		if err != nil {
			return nil, err
		}
		out[k] = store.EncValue{Ciphertext: ct}
	}
	return out, nil
}

// --- base snapshot ---

// LoadBase reads the plaintext base snapshot, returning an empty one if absent.
func (e *Engine) LoadBase() (store.Plaintext, error) {
	b, err := os.ReadFile(gitutil.BasePath(e.GitDir))
	if err != nil {
		if os.IsNotExist(err) {
			return store.Plaintext{}, nil
		}
		return nil, err
	}
	var base store.Base
	if err := json.Unmarshal(b, &base); err != nil {
		return nil, fmt.Errorf("parsing base snapshot: %w", err)
	}
	if base.Data == nil {
		base.Data = store.Plaintext{}
	}
	return base.Data, nil
}

// SaveBase writes the plaintext base snapshot into .git/gitward.
func (e *Engine) SaveBase(data store.Plaintext) error {
	path := gitutil.BasePath(e.GitDir)
	if err := gitutil.EnsureDir(path); err != nil {
		return err
	}
	b, err := json.MarshalIndent(store.Base{Version: 1, Data: data}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// --- leaf IO ---

// ReadLeaf reads and parses a leaf file, returning nil (not an error) when it
// does not exist.
func (e *Engine) ReadLeaf(basepath string, tier store.Tier, env string) (map[string]string, bool, error) {
	full := filepath.Join(e.Root, leaf.Path(basepath, tier, env))
	b, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	m, err := leaf.Parse(string(b))
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", full, err)
	}
	return m, true, nil
}

// WriteLeaf serializes vals to the leaf file. When vals is empty the file is
// removed so a fully-deleted tier leaves no stale file.
func (e *Engine) WriteLeaf(basepath string, tier store.Tier, env string, vals map[string]string) error {
	full := filepath.Join(e.Root, leaf.Path(basepath, tier, env))
	if len(vals) == 0 {
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(leaf.Serialize(vals)), 0o600)
}

// --- reconciliation ---

// Cell identifies one (path, env, tier) location.
type Cell struct {
	Path string
	Env  string
	Tier store.Tier
}

func (c Cell) String() string { return fmt.Sprintf("%s [%s/%s]", c.Path, c.Env, c.Tier) }

// CellResult pairs a cell with its per-key merge results.
type CellResult struct {
	Cell    Cell
	Results []merge.Result
}

// Plan computes merge results for every registered cell by combining the
// decrypted store, the leaf files on disk, and the base snapshot.
func (e *Engine) Plan() ([]CellResult, error) {
	sp, err := e.DecryptStore()
	if err != nil {
		return nil, err
	}
	base, err := e.LoadBase()
	if err != nil {
		return nil, err
	}

	// Collect the union of cells present in store or base (leaves are always a
	// subset of registered store cells).
	cells := map[Cell]struct{}{}
	collect := func(p store.Plaintext) {
		for path, envs := range p {
			for env, tiers := range envs {
				for tier := range tiers {
					cells[Cell{path, env, tier}] = struct{}{}
				}
			}
		}
	}
	collect(sp)
	collect(base)

	ordered := make([]Cell, 0, len(cells))
	for c := range cells {
		ordered = append(ordered, c)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Path != ordered[j].Path {
			return ordered[i].Path < ordered[j].Path
		}
		if ordered[i].Env != ordered[j].Env {
			return ordered[i].Env < ordered[j].Env
		}
		return ordered[i].Tier < ordered[j].Tier
	})

	var out []CellResult
	for _, c := range ordered {
		leafVals, existed, err := e.ReadLeaf(c.Path, c.Tier, c.Env)
		if err != nil {
			return nil, err
		}
		bv := cellVals(base, c)
		if !existed {
			// A missing leaf file is not a deletion of every key; the file is
			// disposable and gitignored. Treat it as "no local changes" so only
			// store->leaf updates apply and nothing is spuriously deleted.
			leafVals = cloneMap(bv)
		}
		res := merge.Merge(cellVals(sp, c), leafVals, bv)
		sort.Slice(res, func(i, j int) bool { return res[i].Key < res[j].Key })
		out = append(out, CellResult{Cell: c, Results: res})
	}
	return out, nil
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cellVals(p store.Plaintext, c Cell) map[string]string {
	if p[c.Path] == nil || p[c.Path][c.Env] == nil {
		return nil
	}
	return p[c.Path][c.Env][c.Tier]
}

// Direction selects how non-conflicting results are applied.
type Direction int

const (
	// StoreToLeaf regenerates leaf files from the store (post-checkout/merge).
	StoreToLeaf Direction = iota
	// LeafToStore captures leaf edits into the store (pre-commit).
	LeafToStore
	// Both applies incoming updates to leaves and local edits to the store.
	Both
)

// Apply resolves the plan in the requested direction. It returns the number of
// conflicts encountered; when conflicts>0 no side effects that would lose data
// are performed for those cells, and the caller decides whether to block.
func (e *Engine) Apply(plan []CellResult, dir Direction) (conflicts int, err error) {
	// Rebuild plaintext from the plan, then persist store, leaves, and base.
	newStorePT := store.Plaintext{}
	newLeafPT := map[Cell]map[string]string{}

	sp, err := e.DecryptStore()
	if err != nil {
		return 0, err
	}
	base, err := e.LoadBase()
	if err != nil {
		return 0, err
	}

	for _, cr := range plan {
		c := cr.Cell
		storeVals := map[string]string{}
		leafVals := map[string]string{}
		for k, v := range cellVals(sp, c) {
			storeVals[k] = v
		}
		lv, existed, err := e.ReadLeaf(c.Path, c.Tier, c.Env)
		if err != nil {
			return 0, err
		}
		if !existed {
			lv = cloneMap(cellVals(base, c))
		}
		for k, v := range lv {
			leafVals[k] = v
		}

		for _, r := range cr.Results {
			switch r.Op {
			case merge.OpNone:
				// keep whichever side has it (they agree)
			case merge.OpTakeStore:
				leafVals[r.Key] = r.Store.Value()
			case merge.OpTakeLeaf:
				storeVals[r.Key] = r.Leaf.Value()
			case merge.OpDelete:
				delete(storeVals, r.Key)
				delete(leafVals, r.Key)
			case merge.OpConflict:
				conflicts++
				// leave both sides untouched for this key
			}
		}
		setCell(newStorePT, c, storeVals)
		newLeafPT[c] = leafVals
	}

	if conflicts > 0 && dir != StoreToLeaf {
		// For LeafToStore/Both a conflict must block before mutating the store.
		return conflicts, nil
	}

	// Persist store (LeafToStore / Both).
	if dir == LeafToStore || dir == Both {
		if err := e.writeStoreFromPT(newStorePT); err != nil {
			return conflicts, err
		}
	}
	// Persist leaves (StoreToLeaf / Both), skipping cells with conflicts.
	if dir == StoreToLeaf || dir == Both {
		for c, vals := range newLeafPT {
			if cellHasConflict(plan, c) {
				continue
			}
			if err := e.WriteLeaf(c.Path, c.Tier, c.Env, vals); err != nil {
				return conflicts, err
			}
		}
	}

	// Advance base to the reconciled truth for non-conflicting cells only.
	for c, vals := range newLeafPT {
		if cellHasConflict(plan, c) {
			continue
		}
		setCell(base, c, vals)
	}
	if err := e.SaveBase(base); err != nil {
		return conflicts, err
	}
	return conflicts, nil
}

func (e *Engine) writeStoreFromPT(pt store.Plaintext) error {
	targets := map[string]store.Target{}
	for path, envs := range pt {
		tgt := store.Target{}
		for env, tiers := range envs {
			bt, err := e.encTier(tiers[store.Buildtime])
			if err != nil {
				return err
			}
			rt, err := e.encTier(tiers[store.Runtime])
			if err != nil {
				return err
			}
			blk := store.EnvBlock{}
			if len(bt) > 0 {
				blk.Buildtime = bt
			}
			if len(rt) > 0 {
				blk.Runtime = rt
			}
			if len(bt) > 0 || len(rt) > 0 {
				tgt[env] = blk
			}
		}
		if len(tgt) > 0 {
			targets[path] = tgt
		}
	}
	e.Store.Targets = targets
	return e.SaveStore()
}

func setCell(p store.Plaintext, c Cell, vals map[string]string) {
	if p[c.Path] == nil {
		p[c.Path] = map[string]map[store.Tier]map[string]string{}
	}
	if p[c.Path][c.Env] == nil {
		p[c.Path][c.Env] = map[store.Tier]map[string]string{}
	}
	p[c.Path][c.Env][c.Tier] = vals
}

func cellHasConflict(plan []CellResult, c Cell) bool {
	for _, cr := range plan {
		if cr.Cell == c {
			return merge.HasConflict(cr.Results)
		}
	}
	return false
}

// StageStore adds the store file to the git index so pre-commit-captured edits
// are included in the in-flight commit.
func (e *Engine) StageStore() error {
	return gitutil.Stage(e.Root, ".gitward.json")
}
