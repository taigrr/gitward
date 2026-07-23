package engine

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/taigrr/gitward/internal/store"
)

// discovered describes a leaf file found on disk and the cell it maps to.
type discovered struct {
	Cell Cell
	Vals map[string]string
}

// leafFilePattern matches generated leaf files and extracts (tier, env).
// Recognized: .env, .env.<env>, .dev.vars, .dev.vars.<env>. Files ending in
// .example are ignored (they are the only committed, human-authored dotfiles).
func classify(name string) (tier store.Tier, env string, ok bool) {
	if strings.HasSuffix(name, ".example") {
		return "", "", false
	}
	switch {
	case name == ".dev.vars":
		return store.Runtime, store.DefaultEnv, true
	case strings.HasPrefix(name, ".dev.vars."):
		return store.Runtime, strings.TrimPrefix(name, ".dev.vars."), true
	case name == ".env":
		return store.Buildtime, store.DefaultEnv, true
	case strings.HasPrefix(name, ".env."):
		return store.Buildtime, strings.TrimPrefix(name, ".env."), true
	}
	return "", "", false
}

// ScanLeaves walks the repo (starting at rel, or the whole repo when empty) for
// leaf dotfiles and returns the cells and parsed values found. Directories
// commonly excluded from secret scanning are skipped.
func (e *Engine) ScanLeaves(rel string) ([]discovered, error) {
	start := e.Root
	if rel != "" {
		start = filepath.Join(e.Root, rel)
	}
	var out []discovered
	err := filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", ".turbo", ".next", "vendor":
				return fs.SkipDir
			}
			return nil
		}
		tier, env, ok := classify(d.Name())
		if !ok {
			return nil
		}
		basepath, err := filepath.Rel(e.Root, filepath.Dir(path))
		if err != nil {
			return err
		}
		vals, existed, err := e.ReadLeaf(basepath, tier, env)
		if err != nil || !existed {
			return err
		}
		out = append(out, discovered{Cell: Cell{basepath, env, tier}, Vals: vals})
		return nil
	})
	return out, err
}

// Register scans for leaf files and adds any keys not already in the store,
// returning the number of keys newly registered. New cells and new keys are
// added; existing keys are left to the normal sync/merge path.
func (e *Engine) Register(rel string) (int, error) {
	found, err := e.ScanLeaves(rel)
	if err != nil {
		return 0, err
	}
	sp, err := e.DecryptStore()
	if err != nil {
		return 0, err
	}
	added := 0
	for _, f := range found {
		cur := cellVals(sp, f.Cell)
		merged := map[string]string{}
		for k, v := range cur {
			merged[k] = v
		}
		for k, v := range e.stripIgnored(f.Cell, f.Vals) {
			if _, exists := merged[k]; !exists {
				merged[k] = v
				added++
			}
		}
		setCell(sp, f.Cell, merged)
	}
	if added == 0 {
		return 0, nil
	}
	if err := e.writeStoreFromPT(sp); err != nil {
		return 0, err
	}
	// advance base for the registered values so they are not seen as drift.
	base, err := e.LoadBase()
	if err != nil {
		return 0, err
	}
	for _, f := range found {
		base_ := cellVals(base, f.Cell)
		merged := map[string]string{}
		for k, v := range base_ {
			merged[k] = v
		}
		for k, v := range cellVals(sp, f.Cell) {
			merged[k] = v
		}
		setCell(base, f.Cell, merged)
	}
	if err := e.SaveBase(base); err != nil {
		return 0, err
	}
	return added, nil
}
