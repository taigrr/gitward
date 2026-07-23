package engine

import (
	"github.com/taigrr/gitward/internal/store"
)

// Ignore marks key as ward-ignored for the cell (path, env, tier): a marker is
// recorded in the store (no value), the key is evicted from the store and base
// snapshot, and the leaf is regenerated so the variable moves into the trailing
// ignored block (preserved, never managed again). Ignoring is per-env.
func (e *Engine) Ignore(c Cell, key string) error {
	if e.Store.Ignored(c.Path, c.Env, c.Tier, key) {
		return nil
	}

	sp, err := e.DecryptStore()
	if err != nil {
		return err
	}
	base, err := e.LoadBase()
	if err != nil {
		return err
	}

	// Capture the last-known value (store first, then base) so it is preserved
	// into the leaf's ignored block even when the leaf file is absent on disk.
	var (
		seed    string
		hasSeed bool
	)
	sv := cloneMap(cellVals(sp, c))
	bv := cloneMap(cellVals(base, c))
	if v, ok := sv[key]; ok {
		seed, hasSeed = v, true
	} else if v, ok := bv[key]; ok {
		seed, hasSeed = v, true
	}
	delete(sv, key)
	setCell(sp, c, sv)
	delete(bv, key)
	setCell(base, c, bv)

	// Record the marker, then persist (writeStoreFromPT re-injects markers).
	e.setIgnoreMarker(c, key)
	if err := e.writeStoreFromPT(sp); err != nil {
		return err
	}
	if err := e.SaveBase(base); err != nil {
		return err
	}

	var seedIgnored map[string]string
	if hasSeed {
		seedIgnored = map[string]string{key: seed}
	}
	// Preserve locally-edited managed values by regenerating from the on-disk
	// leaf when present; fall back to the store otherwise.
	managed := cellVals(sp, c)
	if lv, existed, err := e.ReadLeaf(c.Path, c.Tier, c.Env); err != nil {
		return err
	} else if existed {
		managed = lv
	}
	return e.writeLeaf(c.Path, c.Tier, c.Env, managed, seedIgnored)
}

// Unignore clears the ignore marker for the cell key. The variable, if still
// present in a leaf file, is captured back into the store on the next sync.
func (e *Engine) Unignore(c Cell, key string) error {
	if !e.Store.Ignored(c.Path, c.Env, c.Tier, key) {
		return nil
	}
	tm := e.storeTierMap(c)
	delete(tm, key)
	e.pruneCell(c)
	return e.SaveStore()
}

// IgnoredKeys returns the sorted ignored names for the cell.
func (e *Engine) IgnoredKeys(c Cell) []string {
	return e.Store.IgnoredNames(c.Path, c.Env, c.Tier)
}

// storeTierMap returns the store TierMap for a cell, or nil if absent.
func (e *Engine) storeTierMap(c Cell) store.TierMap {
	blk, ok := e.Store.Targets[c.Path][c.Env]
	if !ok {
		return nil
	}
	if c.Tier == store.Runtime {
		return blk.Runtime
	}
	return blk.Buildtime
}

// setIgnoreMarker writes an Ignore marker into the store cell, creating the
// target/env/tier maps as needed.
func (e *Engine) setIgnoreMarker(c Cell, key string) {
	if e.Store.Targets == nil {
		e.Store.Targets = map[string]store.Target{}
	}
	if e.Store.Targets[c.Path] == nil {
		e.Store.Targets[c.Path] = store.Target{}
	}
	blk := e.Store.Targets[c.Path][c.Env]
	if c.Tier == store.Runtime {
		if blk.Runtime == nil {
			blk.Runtime = store.TierMap{}
		}
		blk.Runtime[key] = store.EncValue{Ignore: true}
	} else {
		if blk.Buildtime == nil {
			blk.Buildtime = store.TierMap{}
		}
		blk.Buildtime[key] = store.EncValue{Ignore: true}
	}
	e.Store.Targets[c.Path][c.Env] = blk
}

// pruneCell removes now-empty tier/env/target maps for a cell so the store does
// not accumulate empty scaffolding.
func (e *Engine) pruneCell(c Cell) {
	tgt, ok := e.Store.Targets[c.Path]
	if !ok {
		return
	}
	blk := tgt[c.Env]
	if len(blk.Buildtime) == 0 {
		blk.Buildtime = nil
	}
	if len(blk.Runtime) == 0 {
		blk.Runtime = nil
	}
	if blk.Buildtime == nil && blk.Runtime == nil {
		delete(tgt, c.Env)
	} else {
		tgt[c.Env] = blk
	}
	if len(tgt) == 0 {
		delete(e.Store.Targets, c.Path)
	}
}
