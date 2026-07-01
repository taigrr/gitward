package crypto

import "testing"

func TestValueRoundTrip(t *testing.T) {
	dek, err := NewDEK()
	if err != nil {
		t.Fatal(err)
	}
	ct, err := EncryptValueStable(dek, "API_KEY", "s3cr3t-value")
	if err != nil {
		t.Fatal(err)
	}
	pt, err := DecryptValue(dek, "API_KEY", ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "s3cr3t-value" {
		t.Fatalf("round trip mismatch: %q", pt)
	}
}

func TestEncryptStable_SameInputsSameCiphertext(t *testing.T) {
	dek, _ := NewDEK()
	a, _ := EncryptValueStable(dek, "K", "v")
	b, _ := EncryptValueStable(dek, "K", "v")
	if a != b {
		t.Fatal("stable encryption must be deterministic to keep git diffs minimal")
	}
	// Different value -> different ciphertext.
	c, _ := EncryptValueStable(dek, "K", "v2")
	if a == c {
		t.Fatal("different plaintext must produce different ciphertext")
	}
}

func TestValue_NameBoundAsAAD(t *testing.T) {
	dek, _ := NewDEK()
	ct, _ := EncryptValueStable(dek, "NAME_A", "v")
	// Moving ciphertext to a different key name must fail authentication.
	if _, err := DecryptValue(dek, "NAME_B", ct); err == nil {
		t.Fatal("decrypt under wrong name should fail (AAD binding)")
	}
}

func TestDEKPassphraseWrapRoundTrip(t *testing.T) {
	dek, _ := NewDEK()
	wrapped, err := WrapDEKPassphrase(dek, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnwrapDEKPassphrase(wrapped, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(dek) {
		t.Fatal("passphrase unwrap did not recover DEK")
	}
	if _, err := UnwrapDEKPassphrase(wrapped, "wrong"); err == nil {
		t.Fatal("wrong passphrase must fail")
	}
}

func TestWrapDEKAge_X25519RoundTrip(t *testing.T) {
	// Use a native age identity so the test needs no ssh key on disk.
	id, err := ageGenerate()
	if err != nil {
		t.Fatal(err)
	}
	dek, _ := NewDEK()
	wrapped, err := WrapDEKAge(dek, []string{id.recipient})
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnwrapDEKAge(wrapped, id.identities())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(dek) {
		t.Fatal("age unwrap did not recover DEK")
	}
}
