package cli

import (
	"fmt"

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
