package crypto

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// GPGAvailable reports whether a gpg binary is on PATH.
func GPGAvailable() bool {
	_, err := exec.LookPath("gpg")
	return err == nil
}

// WrapDEKPGP encrypts the DEK to the given gpg recipients (key IDs or
// fingerprints), producing ASCII-armored output for Keys.DEKPGP. Encryption
// needs only public keys, so no agent interaction occurs here.
func WrapDEKPGP(dek []byte, recipients []string) (string, error) {
	if len(recipients) == 0 {
		return "", fmt.Errorf("no gpg recipients")
	}
	args := []string{"--batch", "--yes", "--armor", "--trust-model", "always", "--encrypt"}
	for _, r := range recipients {
		args = append(args, "--recipient", r)
	}
	cmd := exec.Command("gpg", args...)
	cmd.Stdin = bytes.NewReader(dek)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gpg encrypt: %w: %s", err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// UnwrapDEKPGP recovers the DEK from ASCII-armored gpg ciphertext, using
// gpg-agent for private-key access (pinentry / cached passphrase). This is the
// agent-backed decryption path for devs who prefer gpg over on-disk ssh keys.
//
// --no-tty prevents gpg from grabbing the terminal when invoked from a git
// hook; the agent still drives pinentry through its own channel. The first
// decrypt of a session may therefore trigger a pinentry prompt; subsequent
// ones use the agent's cached passphrase.
func UnwrapDEKPGP(armored string) ([]byte, error) {
	cmd := exec.Command("gpg", "--batch", "--yes", "--no-tty", "--decrypt")
	cmd.Stdin = strings.NewReader(armored)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gpg decrypt: %w: %s", err, strings.TrimSpace(errb.String()))
	}
	dek := out.Bytes()
	if len(dek) != DEKSize {
		return nil, fmt.Errorf("unwrapped DEK has wrong length %d", len(dek))
	}
	return dek, nil
}
