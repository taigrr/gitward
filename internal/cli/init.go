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
		Long:  "Create $GIT_ROOT/.gitward.json with a fresh data key wrapped to the given\nrecipients. Recipients are grouped by name (typically a GitHub username).\nProvide GitHub usernames (their public ssh keys are fetched), explicit\nname=key ssh/age recipients, name=fpr gpg recipients, and/or a shared\npassphrase (read from GITWARD_PASSPHRASE, used for CI and break-glass).",
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			recipients := map[string][]string{}
			for _, u := range githubUsers {
				keys, err := engine.FetchGitHubKeys(u)
				if err != nil {
					return err
				}
				c.Printf("fetched %d key(s) for %s\n", len(keys), u)
				recipients[u] = append(recipients[u], keys...)
			}
			for _, spec := range append(append([]string{}, sshRecipients...), gpgRecipients...) {
				name, key, err := parseNamedRecipient(spec)
				if err != nil {
					return err
				}
				recipients[name] = append(recipients[name], key)
			}
			pass := ""
			if withPassphrase {
				pass = os.Getenv("GITWARD_PASSPHRASE")
				if pass == "" {
					return fmt.Errorf("--passphrase set but GITWARD_PASSPHRASE is empty")
				}
			}
			if err := e.Init(recipients, pass); err != nil {
				return err
			}
			c.Println("initialized .gitward.json")
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&githubUsers, "github", nil, "GitHub usernames whose ssh keys become recipients")
	cmd.Flags().StringSliceVar(&sshRecipients, "ssh-recipient", nil, "explicit age/ssh recipient as name=<line>")
	cmd.Flags().StringSliceVar(&gpgRecipients, "gpg", nil, "gpg recipient as name=<fingerprint>")
	cmd.Flags().BoolVar(&withPassphrase, "passphrase", false, "also wrap the data key with GITWARD_PASSPHRASE (CI/break-glass)")
	addSSHFlag(cmd)
	return cmd
}

// parseNamedRecipient splits a "name=key" spec. The key itself may contain '='
// (unlikely for ssh/gpg), so only the first '=' is used as the separator.
func parseNamedRecipient(spec string) (name, key string, err error) {
	i := strings.IndexByte(spec, '=')
	if i <= 0 || i == len(spec)-1 {
		return "", "", fmt.Errorf("recipient %q must be in the form name=key", spec)
	}
	return strings.TrimSpace(spec[:i]), strings.TrimSpace(spec[i+1:]), nil
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
