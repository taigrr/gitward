package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/taigrr/gitward/internal/crypto"
	"github.com/taigrr/gitward/internal/engine"
)

// NewSetPassphraseCmd rotates (or establishes) the shared scrypt passphrase
// wrap of the data key, leaving the age and gpg wraps untouched.
func NewSetPassphraseCmd() *cobra.Command {
	var fromStdin bool
	cmd := &cobra.Command{
		Use:   "set-passphrase",
		Short: "Rotate the shared passphrase (GITWARD_PASSPHRASE) wrap of the data key",
		Long: "Re-wrap the data key under a new shared scrypt passphrase (the one CI and\n" +
			"break-glass recovery use), replacing any existing passphrase wrap and\n" +
			"leaving the age and gpg recipient wraps untouched.\n\n" +
			"The new passphrase is read, in order, from: --stdin (raw, trailing newline\n" +
			"stripped); the GITWARD_PASSPHRASE environment variable; or an interactive\n" +
			"prompt with confirmation. Unlocking the store to recover the data key still\n" +
			"uses any available method (ssh/age, gpg, or the current passphrase), so you\n" +
			"can set GITWARD_PASSPHRASE to the NEW value and still rotate while your ssh\n" +
			"or gpg key performs the unlock.\n\n" +
			"After rotating, update the GITWARD_PASSPHRASE secret wherever it is stored\n" +
			"(CI, secret manager) and commit the changed .gitward.json.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return errNotInitialized
			}
			pass, err := readNewPassphrase(c, fromStdin)
			if err != nil {
				return err
			}
			if err := e.SetPassphrase(pass); err != nil {
				return err
			}
			if err := e.SaveStore(); err != nil {
				return err
			}
			if jsonFlag {
				return printJSON(c, struct {
					Passphrase bool `json:"passphrase"`
				}{true})
			}
			c.Println("rotated passphrase wrap; update the GITWARD_PASSPHRASE secret and commit .gitward.json")
			return nil
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read the new passphrase from stdin (raw, trailing newline stripped)")
	addSSHFlag(cmd)
	return cmd
}

// NewRmPassphraseCmd removes the shared passphrase wrap.
func NewRmPassphraseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm-passphrase",
		Short: "Remove the shared passphrase (GITWARD_PASSPHRASE) wrap of the data key",
		Long: "Remove the shared scrypt passphrase wrap, leaving the age and gpg recipient\n" +
			"wraps intact. Refuses to remove the only remaining wrap. After removing it,\n" +
			"CI and break-glass recovery via GITWARD_PASSPHRASE stop working.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return errNotInitialized
			}
			removed, err := e.ClearPassphrase()
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("no passphrase wrap present")
			}
			if err := e.SaveStore(); err != nil {
				return err
			}
			if jsonFlag {
				return printJSON(c, struct {
					Passphrase bool `json:"passphrase"`
				}{false})
			}
			c.Println("removed passphrase wrap; commit .gitward.json")
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// readNewPassphrase resolves the new passphrase from stdin, the environment, or
// an interactive prompt (in that order). The result is validated to be
// non-empty.
func readNewPassphrase(c *cobra.Command, fromStdin bool) (string, error) {
	if fromStdin {
		b, err := io.ReadAll(c.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("reading passphrase from stdin: %w", err)
		}
		pass := strings.TrimRight(string(b), "\r\n")
		if pass == "" {
			return "", fmt.Errorf("empty passphrase on stdin")
		}
		return pass, nil
	}
	if pass := os.Getenv(crypto.PassphraseEnv); pass != "" {
		return pass, nil
	}
	return promptPassphraseConfirmed()
}

// promptPassphraseConfirmed reads a passphrase twice from the terminal without
// echo and fails unless both entries match and are non-empty.
func promptPassphraseConfirmed() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("no passphrase source: set %s, pass --stdin, or run interactively", crypto.PassphraseEnv)
	}
	fmt.Fprint(os.Stderr, "New passphrase: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("reading passphrase: %w", err)
	}
	fmt.Fprint(os.Stderr, "Confirm passphrase: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("reading passphrase: %w", err)
	}
	if string(first) != string(second) {
		return "", fmt.Errorf("passphrases do not match")
	}
	if len(first) == 0 {
		return "", fmt.Errorf("empty passphrase")
	}
	return string(first), nil
}
