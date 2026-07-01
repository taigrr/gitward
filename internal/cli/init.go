package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
)

// NewInitCmd creates the store and data key, wrapping it to recipients.
func NewInitCmd() *cobra.Command {
	var githubUsers []string
	var sshRecipients []string
	var gpgRecipients []string
	var withPassphrase bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create the encrypted store and data key",
		Long:  "Create $GIT_ROOT/.gitward.json with a fresh data key wrapped to the given\nrecipients. Provide age/ssh recipients directly, GitHub usernames (their\npublic ssh keys are fetched), gpg fingerprints, and/or a shared passphrase\n(read from GITWARD_PASSPHRASE, used for CI and break-glass).",
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			age := append([]string{}, sshRecipients...)
			for _, u := range githubUsers {
				keys, err := engine.FetchGitHubKeys(u)
				if err != nil {
					return err
				}
				c.Printf("fetched %d key(s) for %s\n", len(keys), u)
				age = append(age, keys...)
			}
			pass := ""
			if withPassphrase {
				pass = os.Getenv("GITWARD_PASSPHRASE")
				if pass == "" {
					return fmt.Errorf("--passphrase set but GITWARD_PASSPHRASE is empty")
				}
			}
			if err := e.Init(age, gpgRecipients, pass); err != nil {
				return err
			}
			c.Println("initialized .gitward.json")
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&githubUsers, "github", nil, "GitHub usernames whose ssh keys become recipients")
	cmd.Flags().StringSliceVar(&sshRecipients, "ssh-recipient", nil, "explicit age/ssh recipient lines")
	cmd.Flags().StringSliceVar(&gpgRecipients, "gpg", nil, "gpg recipient fingerprints/key ids")
	cmd.Flags().BoolVar(&withPassphrase, "passphrase", false, "also wrap the data key with GITWARD_PASSPHRASE (CI/break-glass)")
	addSSHFlag(cmd)
	return cmd
}

func promptLine(prompt string) (string, error) {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
