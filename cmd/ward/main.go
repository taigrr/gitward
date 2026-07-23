// Command ward is the gitward CLI: it manages a single encrypted secret store
// at $GIT_ROOT/.gitward.json and fans its values out to leaf .env / .dev.vars
// files, capturing local edits back on commit via git hooks.
package main

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/cli"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:           "ward",
		Short:         "Painless, git-tracked, encrypted secrets for your monorepo",
		Long:          "ward (gitward) keeps one encrypted store at $GIT_ROOT/.gitward.json and\ngenerates leaf .env / .dev.vars files from it, capturing your local edits\nback into the store on commit. Secrets stay in git but encrypted; a leaked\nrepo leaks no plaintext.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		cli.NewInitCmd(),
		cli.NewStatusCmd(),
		cli.NewDiffCmd(),
		cli.NewSyncCmd(),
		cli.NewRegisterCmd(),
		cli.NewEditCmd(),
		cli.NewListCmd(),
		cli.NewIgnoreCmd(),
		cli.NewUnignoreCmd(),
		cli.NewResolveCmd(),
		cli.NewInstallCmd(),
		cli.NewAddRecipientCmd(),
		cli.NewRmRecipientCmd(),
		cli.NewDoctorCmd(),
		cli.NewHookCmd(),
	)

	if err := fang.Execute(context.Background(), root, fang.WithVersion(version)); err != nil {
		os.Exit(1)
	}
}
