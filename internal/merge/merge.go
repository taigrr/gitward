// Package merge implements the lossless 3-way reconciliation between the
// encrypted store (S), the leaf .env files on disk (L), and the base snapshot
// (B = the plaintext from the last successful sync, kept in .git/gitward).
//
// The base is what makes resolution lossless: without it we cannot tell "the
// dev edited this value" from "an incoming commit changed this value", and we
// are forced into either store-always-wins (clobbers local edits) or
// leaf-always-wins (clobbers pulls). With B we apply git-style merge rules:
//
//	S vs B    L vs B          result
//	------    ------          ------
//	same      same            no-op
//	changed   same            take store  (incoming update -> rewrite leaf)
//	same      changed         take leaf   (dev edit    -> update store)
//	changed   changed, ==     agree       (converged, just advance base)
//	changed   changed, !=     CONFLICT    (block / prompt)
//	added S   absent L        take store
//	absent S  added L         take leaf
//	removed S same as B in L  delete      (removed upstream)
//	same as B removed L       delete      (dev deleted -> remove from store)
//	removed   removed         delete
//	removed   changed         CONFLICT
//	changed   removed         CONFLICT
package merge

// Op is the resolved action for a single key.
type Op int

const (
	// OpNone means store, leaf, and base already agree.
	OpNone Op = iota
	// OpTakeStore rewrites the leaf value from the store (incoming update).
	OpTakeStore
	// OpTakeLeaf updates the store value from the leaf (local edit).
	OpTakeLeaf
	// OpDelete removes the key from both sides.
	OpDelete
	// OpConflict means both sides changed to different values.
	OpConflict
)

// Val is the presence and value of a key in one of the three sources.
type Val struct {
	set bool
	v   string
}

// Value returns the string value (empty when unset).
func (v Val) Value() string { return v.v }

// Set reports whether the value is present in the source.
func (v Val) Set() bool { return v.set }

func present(m map[string]string, k string) Val {
	if m == nil {
		return Val{}
	}
	if v, ok := m[k]; ok {
		return Val{set: true, v: v}
	}
	return Val{}
}

// Result is the merge decision for one key.
type Result struct {
	Key   string
	Op    Op
	Store Val // value currently in store (if set)
	Leaf  Val // value currently in leaf (if set)
	Base  Val // value in base snapshot (if set)
}

// Conflict reports whether the result requires human resolution.
func (r Result) Conflict() bool { return r.Op == OpConflict }

// keyOf3 resolves a single key given its store/leaf/base presence.
func decide(k string, s, l, b Val) Result {
	r := Result{Key: k, Store: s, Leaf: l, Base: b}

	storeChanged := s.set != b.set || (s.set && s.v != b.v)
	leafChanged := l.set != b.set || (l.set && l.v != b.v)

	switch {
	case !storeChanged && !leafChanged:
		r.Op = OpNone
	case storeChanged && !leafChanged:
		// Incoming update. Deletion upstream propagates as delete.
		if !s.set {
			r.Op = OpDelete
		} else {
			r.Op = OpTakeStore
		}
	case !storeChanged && leafChanged:
		// Local edit. Local deletion propagates as delete.
		if !l.set {
			r.Op = OpDelete
		} else {
			r.Op = OpTakeLeaf
		}
	default: // both changed
		switch {
		case s.set && l.set && s.v == l.v:
			r.Op = OpNone // converged to same value
		case !s.set && !l.set:
			r.Op = OpDelete // both deleted
		default:
			r.Op = OpConflict
		}
	}
	return r
}

// Merge reconciles one flat key/value map from each of the three sources and
// returns a stable, deterministic slice of per-key results (sorted by key by
// the caller if needed). It never mutates its inputs.
func Merge(storeVals, leafVals, baseVals map[string]string) []Result {
	seen := map[string]struct{}{}
	for k := range storeVals {
		seen[k] = struct{}{}
	}
	for k := range leafVals {
		seen[k] = struct{}{}
	}
	for k := range baseVals {
		seen[k] = struct{}{}
	}
	out := make([]Result, 0, len(seen))
	for k := range seen {
		out = append(out, decide(k, present(storeVals, k), present(leafVals, k), present(baseVals, k)))
	}
	return out
}

// HasConflict reports whether any result is a conflict.
func HasConflict(rs []Result) bool {
	for _, r := range rs {
		if r.Conflict() {
			return true
		}
	}
	return false
}
