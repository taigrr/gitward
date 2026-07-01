// Package cli defines the ward subcommands, all backed by internal/engine.
package cli

import "github.com/spf13/cobra"

// sshKeyFlag is the optional explicit ssh key path shared across commands.
var sshKeyFlag string

func addSSHFlag(c *cobra.Command) {
	c.Flags().StringVar(&sshKeyFlag, "ssh-key", "", "ssh private key path for decryption (default: ~/.ssh/id_ed25519, id_rsa)")
}
