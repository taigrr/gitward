package cli

import (
	"fmt"
	"strings"

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

// NewAddRecipientCmd adds an age/ssh/github or gpg recipient.
func NewAddRecipientCmd() *cobra.Command {
	var kind string
	cmd := &cobra.Command{
		Use:   "add-recipient <ssh-key-line|gpg-fpr|github-username>",
		Short: "Add a recipient and rewrap the data key",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return fmt.Errorf("store not initialized (run 'ward init')")
			}
			arg := args[0]
			switch {
			case kind == "gpg":
				if err := e.AddPGPRecipient(arg); err != nil {
					return err
				}
				c.Printf("added gpg recipient %s\n", arg)
			case kind == "github" || (!strings.HasPrefix(arg, "ssh-") && !strings.HasPrefix(arg, "age1") && kind == ""):
				keys, err := engine.FetchGitHubKeys(arg)
				if err != nil {
					return err
				}
				for _, k := range keys {
					if err := e.AddAgeRecipient(k); err != nil {
						return err
					}
				}
				c.Printf("added %d ssh key(s) for %s\n", len(keys), arg)
			default:
				if err := e.AddAgeRecipient(arg); err != nil {
					return err
				}
				c.Println("added age/ssh recipient")
			}
			return e.SaveStore()
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "force recipient kind: age|gpg|github (default: inferred)")
	addSSHFlag(cmd)
	return cmd
}

// NewRmRecipientCmd removes a recipient by exact identifier.
func NewRmRecipientCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm-recipient <recipient>",
		Short: "Remove a recipient and rewrap the data key",
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
