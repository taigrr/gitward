package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
)

// NewInstallCmd installs the git hooks.
func NewInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install git hooks (post-checkout, post-merge, pre-commit)",
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if err := e.InstallHooks(); err != nil {
				return err
			}
			c.Println("installed git hooks")
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// NewAddRecipientCmd adds keys for a named recipient and rewraps.
func NewAddRecipientCmd() *cobra.Command {
	var githubUser bool
	cmd := &cobra.Command{
		Use:   "add-recipient <name> [ssh-line|gpg-fpr|age1...]",
		Short: "Add a recipient (by name) and rewrap the data key",
		Long: "Add keys under a recipient name and rewrap the data key.\n\n" +
			"  ward add-recipient alice \"ssh-ed25519 AAAA...\"   # explicit key\n" +
			"  ward add-recipient alice DEADBEEF...              # gpg fingerprint\n" +
			"  ward add-recipient alice --github                # fetch alice's GitHub ssh keys",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return fmt.Errorf("store not initialized (run 'ward init')")
			}
			name := args[0]
			var keys []string
			switch {
			case githubUser:
				fetched, err := engine.FetchGitHubKeys(name)
				if err != nil {
					return err
				}
				keys = fetched
				c.Printf("fetched %d key(s) for %s\n", len(keys), name)
			case len(args) == 2:
				keys = []string{args[1]}
			default:
				return fmt.Errorf("provide a key argument or --github")
			}
			if err := e.AddRecipientKeys(name, keys); err != nil {
				return err
			}
			if err := e.SaveStore(); err != nil {
				return err
			}
			c.Printf("added %d key(s) for %s\n", len(keys), name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&githubUser, "github", false, "treat <name> as a GitHub username and fetch their ssh keys")
	addSSHFlag(cmd)
	return cmd
}

// recipientJSON is one entry in `recipients --json`.
type recipientJSON struct {
	Name string   `json:"name"`
	Keys []string `json:"keys"`
}

// NewRecipientsCmd lists the recipients the data key is wrapped to.
func NewRecipientsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipients",
		Short: "List recipients (names and public keys / fingerprints) of the data key",
		Long: "List every recipient name with its ssh/age public keys or gpg fingerprints,\n" +
			"and whether a passphrase wrap (GITWARD_PASSPHRASE) is present.\n\n" +
			"With --json, emits {passphrase, recipients:[{name, keys}]}.",
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return notInitialized(c)
			}
			recips, hasPass := e.Recipients()
			names := make([]string, 0, len(recips))
			for n := range recips {
				names = append(names, n)
			}
			sort.Strings(names)
			out := make([]recipientJSON, 0, len(names))
			for _, n := range names {
				keys := append([]string{}, recips[n]...)
				sort.Strings(keys)
				out = append(out, recipientJSON{Name: n, Keys: keys})
			}
			if jsonFlag {
				return printJSON(c, struct {
					Passphrase bool            `json:"passphrase"`
					Recipients []recipientJSON `json:"recipients"`
				}{hasPass, out})
			}
			for _, r := range out {
				c.Println(r.Name)
				for _, k := range r.Keys {
					c.Printf("  %s\n", k)
				}
			}
			if hasPass {
				c.Println("(passphrase wrap present)")
			}
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// NewRmRecipientCmd removes a named recipient and rewraps.
func NewRmRecipientCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm-recipient <name>",
		Short: "Remove a recipient (by name) and rewrap the data key",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return fmt.Errorf("store not initialized (run 'ward init')")
			}
			removed, err := e.RemoveRecipient(args[0])
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("recipient %q not found", args[0])
			}
			if err := e.SaveStore(); err != nil {
				return err
			}
			c.Println("removed recipient; note: rotate actual secret values, prior ciphertext remains in git history")
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}
