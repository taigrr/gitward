package engine

import (
	"fmt"

	"github.com/taigrr/gitward/internal/store"
)

// CellPlaintext returns the decrypted values for a single cell.
func (e *Engine) CellPlaintext(c Cell) (map[string]string, error) {
	sp, err := e.DecryptStore()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, v := range cellVals(sp, c) {
		out[k] = v
	}
	return out, nil
}

// SetCellPlaintext replaces a single cell's values in the store, re-encrypts,
// writes the store, regenerates the corresponding leaf, and advances the base
// so the edit is not seen as drift.
func (e *Engine) SetCellPlaintext(c Cell, vals map[string]string) error {
	sp, err := e.DecryptStore()
	if err != nil {
		return err
	}
	setCell(sp, c, vals)
	if err := e.writeStoreFromPT(sp); err != nil {
		return err
	}
	if err := e.WriteLeaf(c.Path, c.Tier, c.Env, vals); err != nil {
		return err
	}
	base, err := e.LoadBase()
	if err != nil {
		return err
	}
	setCell(base, c, vals)
	return e.SaveBase(base)
}

// ResolveKey records a human decision for one conflicting key: it sets both the
// store and the leaf to value (or deletes the key when del is true), then
// advances the base for that key. This clears the conflict.
func (e *Engine) ResolveKey(c Cell, key, value string, del bool) error {
	sp, err := e.DecryptStore()
	if err != nil {
		return err
	}
	vals := map[string]string{}
	for k, v := range cellVals(sp, c) {
		vals[k] = v
	}
	if del {
		delete(vals, key)
	} else {
		vals[key] = value
	}
	return e.SetCellPlaintext(c, vals)
}

// ParseCellArgs validates and builds a Cell from path/env/tier CLI arguments.
func ParseCellArgs(path, env, tier string) (Cell, error) {
	var t store.Tier
	switch tier {
	case "buildtime":
		t = store.Buildtime
	case "runtime":
		t = store.Runtime
	default:
		return Cell{}, fmt.Errorf("tier must be 'buildtime' or 'runtime', got %q", tier)
	}
	if env == "" {
		env = store.DefaultEnv
	}
	return Cell{Path: path, Env: env, Tier: t}, nil
}
