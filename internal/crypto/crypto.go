// Package crypto implements gitward's envelope encryption.
//
// A single random 32-byte data-encryption key (DEK) encrypts every secret
// value with AES-256-GCM. The DEK itself is wrapped independently for each
// recipient class, and ANY one wrap can recover it:
//
//   - age recipients (ssh-ed25519 / ssh-rsa / native age) -> Keys.DEKAge
//   - a shared scrypt passphrase (CI + break-glass)        -> Keys.DEKPassphrase
//   - gpg recipients (via gpg-agent, handled in gpg.go)    -> Keys.DEKPGP
//
// Because values are encrypted with a stable DEK and a random nonce is only
// drawn when plaintext changes (see EncryptValueStable), unchanged values stay
// byte-identical across writes, keeping the JSON store's git diffs minimal.
package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"filippo.io/age/armor"
)

// DEKSize is the data-encryption-key length (AES-256).
const DEKSize = 32

const defaultScryptWorkFactor = 18

var scryptWorkFactor = defaultScryptWorkFactor

// NewDEK returns a fresh random data-encryption key.
func NewDEK() ([]byte, error) {
	k := make([]byte, DEKSize)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		return nil, fmt.Errorf("generating DEK: %w", err)
	}
	return k, nil
}

// aead builds the AES-256-GCM AEAD for a DEK.
func aead(dek []byte) (cipher.AEAD, error) {
	if len(dek) != DEKSize {
		return nil, fmt.Errorf("bad DEK length %d, want %d", len(dek), DEKSize)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// EncryptValueStable encrypts plaintext under the DEK, deriving the GCM nonce
// deterministically from (DEK, key, plaintext). This makes the ciphertext a
// pure function of its inputs: re-encrypting an unchanged value yields the same
// bytes, so the store diff stays empty. The name is bound into the nonce so two
// keys with the same value do not collide, and it is authenticated as
// additional data so a value cannot be moved to a different key.
//
// Deterministic nonces are safe here because the (key,plaintext) tuple that
// derives the nonce is exactly what is being encrypted; a repeated nonce only
// ever occurs for identical plaintext under the same key, which already
// produces identical ciphertext.
func EncryptValueStable(dek []byte, name, plaintext string) (string, error) {
	a, err := aead(dek)
	if err != nil {
		return "", err
	}
	nonce := deriveNonce(dek, name, plaintext, a.NonceSize())
	ct := a.Seal(nil, nonce, []byte(plaintext), []byte(name))
	out := append(nonce, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptValue reverses EncryptValueStable.
func DecryptValue(dek []byte, name, armored string) (string, error) {
	a, err := aead(dek)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(armored)
	if err != nil {
		return "", fmt.Errorf("decoding value for %q: %w", name, err)
	}
	ns := a.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("ciphertext for %q too short", name)
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := a.Open(nil, nonce, ct, []byte(name))
	if err != nil {
		return "", fmt.Errorf("decrypting %q: %w", name, err)
	}
	return string(pt), nil
}

func deriveNonce(dek []byte, name, plaintext string, size int) []byte {
	h := sha256.New()
	h.Write(dek)
	h.Write([]byte{0x00})
	h.Write([]byte(name))
	h.Write([]byte{0x00})
	h.Write([]byte(plaintext))
	sum := h.Sum(nil)
	return sum[:size]
}

// WrapDEKAge encrypts the DEK to a set of age recipients (native age or ssh
// public keys) and returns armored age ciphertext for Keys.DEKAge.
func WrapDEKAge(dek []byte, recipientLines []string) (string, error) {
	recips, err := ParseAgeRecipients(recipientLines)
	if err != nil {
		return "", err
	}
	if len(recips) == 0 {
		return "", fmt.Errorf("no age recipients")
	}
	var buf bytes.Buffer
	armorW := armor.NewWriter(&buf)
	w, err := age.Encrypt(armorW, recips...)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(dek); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	if err := armorW.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// UnwrapDEKAge recovers the DEK from armored age ciphertext using the provided
// identities (e.g. an ssh private key or age identity).
func UnwrapDEKAge(armored string, ids []age.Identity) ([]byte, error) {
	ar := armor.NewReader(bytes.NewReader([]byte(armored)))
	r, err := age.Decrypt(ar, ids...)
	if err != nil {
		return nil, fmt.Errorf("unwrapping DEK (age): %w", err)
	}
	dek, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(dek) != DEKSize {
		return nil, fmt.Errorf("unwrapped DEK has wrong length %d", len(dek))
	}
	return dek, nil
}

// WrapDEKPassphrase encrypts the DEK under a scrypt passphrase recipient for
// CI and break-glass recovery.
func WrapDEKPassphrase(dek []byte, passphrase string) (string, error) {
	r, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return "", err
	}
	r.SetWorkFactor(scryptWorkFactor)
	var buf bytes.Buffer
	armorW := armor.NewWriter(&buf)
	w, err := age.Encrypt(armorW, r)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(dek); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	if err := armorW.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// UnwrapDEKPassphrase recovers the DEK using the shared passphrase.
func UnwrapDEKPassphrase(armored, passphrase string) ([]byte, error) {
	id, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, err
	}
	return UnwrapDEKAge(armored, []age.Identity{id})
}

// ParseAgeRecipients parses a mix of native age recipients ("age1...") and ssh
// public key lines ("ssh-ed25519 AAAA...", "ssh-rsa AAAA...").
func ParseAgeRecipients(lines []string) ([]age.Recipient, error) {
	out := make([]age.Recipient, 0, len(lines))
	for _, line := range lines {
		r, err := parseOneRecipient(line)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func parseOneRecipient(line string) (age.Recipient, error) {
	if len(line) >= 4 && line[:4] == "age1" {
		return age.ParseX25519Recipient(line)
	}
	r, err := agessh.ParseRecipient(line)
	if err != nil {
		return nil, fmt.Errorf("parsing recipient %q: %w", line, err)
	}
	return r, nil
}
