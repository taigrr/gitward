package merge

import (
	"sort"
	"testing"
)

// m is a tiny helper to build maps inline.
func m(pairs ...string) map[string]string {
	if len(pairs)%2 != 0 {
		panic("odd pairs")
	}
	out := map[string]string{}
	for i := 0; i < len(pairs); i += 2 {
		out[pairs[i]] = pairs[i+1]
	}
	return out
}

func find(rs []Result, key string) Result {
	for _, r := range rs {
		if r.Key == key {
			return r
		}
	}
	return Result{Key: key, Op: -1}
}

func TestMerge_NoOp(t *testing.T) {
	rs := Merge(m("A", "1"), m("A", "1"), m("A", "1"))
	if got := find(rs, "A").Op; got != OpNone {
		t.Fatalf("A: want OpNone, got %d", got)
	}
}

func TestMerge_IncomingUpdate_StoreChangedLeafSame(t *testing.T) {
	// store advanced past base, leaf still at base -> take store
	rs := Merge(m("A", "2"), m("A", "1"), m("A", "1"))
	if got := find(rs, "A").Op; got != OpTakeStore {
		t.Fatalf("A: want OpTakeStore, got %d", got)
	}
}

func TestMerge_LocalEdit_LeafChangedStoreSame(t *testing.T) {
	rs := Merge(m("A", "1"), m("A", "9"), m("A", "1"))
	if got := find(rs, "A").Op; got != OpTakeLeaf {
		t.Fatalf("A: want OpTakeLeaf, got %d", got)
	}
	if v := find(rs, "A").Leaf.v; v != "9" {
		t.Fatalf("A: want leaf value 9, got %q", v)
	}
}

func TestMerge_BothChangedSameValue_Converged(t *testing.T) {
	rs := Merge(m("A", "7"), m("A", "7"), m("A", "1"))
	if got := find(rs, "A").Op; got != OpNone {
		t.Fatalf("A: want OpNone (converged), got %d", got)
	}
}

func TestMerge_BothChangedDifferent_Conflict(t *testing.T) {
	rs := Merge(m("A", "2"), m("A", "3"), m("A", "1"))
	r := find(rs, "A")
	if r.Op != OpConflict {
		t.Fatalf("A: want OpConflict, got %d", r.Op)
	}
	if !HasConflict(rs) {
		t.Fatal("HasConflict should be true")
	}
}

func TestMerge_AddedInLeaf(t *testing.T) {
	rs := Merge(nil, m("NEW", "x"), nil)
	if got := find(rs, "NEW").Op; got != OpTakeLeaf {
		t.Fatalf("NEW: want OpTakeLeaf, got %d", got)
	}
}

func TestMerge_AddedInStore(t *testing.T) {
	rs := Merge(m("NEW", "x"), nil, nil)
	if got := find(rs, "NEW").Op; got != OpTakeStore {
		t.Fatalf("NEW: want OpTakeStore, got %d", got)
	}
}

func TestMerge_DeletedInLeaf(t *testing.T) {
	// present in store+base, dev removed from leaf -> delete from store
	rs := Merge(m("A", "1"), nil, m("A", "1"))
	if got := find(rs, "A").Op; got != OpDelete {
		t.Fatalf("A: want OpDelete, got %d", got)
	}
}

func TestMerge_DeletedInStore(t *testing.T) {
	// removed upstream, leaf still has old base value -> delete from leaf
	rs := Merge(nil, m("A", "1"), m("A", "1"))
	if got := find(rs, "A").Op; got != OpDelete {
		t.Fatalf("A: want OpDelete, got %d", got)
	}
}

func TestMerge_DeletedBoth(t *testing.T) {
	rs := Merge(nil, nil, m("A", "1"))
	if got := find(rs, "A").Op; got != OpDelete {
		t.Fatalf("A: want OpDelete, got %d", got)
	}
}

func TestMerge_DeleteVsEdit_Conflict(t *testing.T) {
	// store removed, leaf edited to new value -> conflict
	rs := Merge(nil, m("A", "9"), m("A", "1"))
	if got := find(rs, "A").Op; got != OpConflict {
		t.Fatalf("A: want OpConflict, got %d", got)
	}
	// mirror: store edited, leaf removed -> conflict
	rs = Merge(m("A", "9"), nil, m("A", "1"))
	if got := find(rs, "A").Op; got != OpConflict {
		t.Fatalf("A(mirror): want OpConflict, got %d", got)
	}
}

func TestMerge_Deterministic_AllKeysPresent(t *testing.T) {
	rs := Merge(m("A", "1", "B", "2"), m("B", "2", "C", "3"), m("A", "1"))
	keys := make([]string, len(rs))
	for i, r := range rs {
		keys[i] = r.Key
	}
	sort.Strings(keys)
	want := []string{"A", "B", "C"}
	if len(keys) != 3 || keys[0] != want[0] || keys[1] != want[1] || keys[2] != want[2] {
		t.Fatalf("want keys %v, got %v", want, keys)
	}
}
