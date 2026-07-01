package leaf

import (
	"testing"

	"github.com/taigrr/gitward/internal/store"
)

func TestFileName(t *testing.T) {
	cases := []struct {
		tier store.Tier
		env  string
		want string
	}{
		{store.Buildtime, "production", ".env.production"},
		{store.Runtime, "production", ".dev.vars.production"},
		{store.Buildtime, "_", ".env"},
		{store.Runtime, "_", ".dev.vars"},
		{store.Buildtime, "", ".env"},
	}
	for _, c := range cases {
		if got := FileName(c.tier, c.env); got != c.want {
			t.Errorf("FileName(%q,%q)=%q want %q", c.tier, c.env, got, c.want)
		}
	}
}

func TestParseRoundTrip(t *testing.T) {
	in := "# comment\nexport A=1\nB=\"two words\"\nC='quoted'\n\n"
	got, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if got["A"] != "1" || got["B"] != "two words" || got["C"] != "quoted" {
		t.Fatalf("parsed wrong: %#v", got)
	}
	if _, ok := got["#"]; ok {
		t.Fatal("comment leaked into map")
	}
}

func TestParseError(t *testing.T) {
	if _, err := Parse("NOEQUALS\n"); err == nil {
		t.Fatal("expected error for line without '='")
	}
}

func TestSerializeDeterministic(t *testing.T) {
	s1 := Serialize(map[string]string{"B": "2", "A": "1"})
	s2 := Serialize(map[string]string{"A": "1", "B": "2"})
	if s1 != s2 {
		t.Fatalf("Serialize not deterministic:\n%q\n%q", s1, s2)
	}
	// A should sort before B
	reparsed, err := Parse(s1)
	if err != nil {
		t.Fatal(err)
	}
	if reparsed["A"] != "1" || reparsed["B"] != "2" {
		t.Fatalf("round trip lost data: %#v", reparsed)
	}
}

func TestQuoteWhitespace(t *testing.T) {
	s := Serialize(map[string]string{"K": "a b"})
	got, _ := Parse(s)
	if got["K"] != "a b" {
		t.Fatalf("whitespace value round trip failed: %q", got["K"])
	}
}
