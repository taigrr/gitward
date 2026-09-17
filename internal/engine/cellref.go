package engine

import (
	"fmt"
	"path/filepath"

	"github.com/taigrr/gitward/internal/leaf"
	"github.com/taigrr/gitward/internal/merge"
)

// ParseCellFile builds a Cell from a leaf file path relative to the repo root
// (e.g. "apps/web/.env.production"), deriving env and tier from the filename
// convention. The file need not exist.
func ParseCellFile(rel string) (Cell, error) {
	name := filepath.Base(rel)
	tier, env, ok := classify(name)
	if !ok {
		return Cell{}, fmt.Errorf("%q is not a leaf file name (.env[.<env>] or .dev.vars[.<env>])", name)
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		dir = ""
	}
	return Cell{Path: dir, Env: env, Tier: tier}, nil
}

// IsLeafFileName reports whether name follows the leaf filename convention.
func IsLeafFileName(name string) bool {
	_, _, ok := classify(filepath.Base(name))
	return ok
}

// File returns the cell's leaf file path relative to the repo root.
func (c Cell) File() string { return leaf.Path(c.Path, c.Tier, c.Env) }

// CellDirty reports whether the plan holds any pending operation (incoming,
// local, delete, or conflict) for the given cell.
func CellDirty(plan []CellResult, c Cell) bool {
	for _, cr := range plan {
		if cr.Cell != c {
			continue
		}
		for _, r := range cr.Results {
			if r.Op != 0 {
				return true
			}
		}
	}
	return false
}

// CellHasLocalChanges reports whether the plan holds a pending local edit or
// conflict for the given cell, i.e. state that a direct store write would
// discard. Incoming-only cells (store ahead of an absent or stale leaf) are
// NOT reported, so a direct write on a fresh clone is allowed.
func CellHasLocalChanges(plan []CellResult, c Cell) bool {
	for _, cr := range plan {
		if cr.Cell != c {
			continue
		}
		for _, r := range cr.Results {
			if r.Op == merge.OpTakeLeaf || r.Op == merge.OpConflict {
				return true
			}
		}
	}
	return false
}

// SetKeys assigns values for one or more keys in a cell, leaving other keys
// untouched, then persists store, leaf, and base via SetCellPlaintext.
func (e *Engine) SetKeys(c Cell, kv map[string]string) error {
	vals, err := e.CellPlaintext(c)
	if err != nil {
		return err
	}
	for k, v := range kv {
		vals[k] = v
	}
	return e.SetCellPlaintext(c, vals)
}

// UnsetKeys removes keys from a cell, returning the number that were present,
// then persists store, leaf, and base via SetCellPlaintext.
func (e *Engine) UnsetKeys(c Cell, keys []string) (int, error) {
	vals, err := e.CellPlaintext(c)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, k := range keys {
		if _, ok := vals[k]; ok {
			delete(vals, k)
			removed++
		}
	}
	if removed == 0 {
		return 0, nil
	}
	return removed, e.SetCellPlaintext(c, vals)
}
