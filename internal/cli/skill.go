package cli

import (
	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/skill"
)

// NewSkillCmd prints the embedded Agent Skills SKILL.md to stdout.
func NewSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the agent skill (SKILL.md) describing how to drive ward",
		Long: "Print ward's Agent Skills definition (https://agentskills.io) to stdout. It\n" +
			"documents both the interactive commands for humans and the non-interactive\n" +
			"JSON commands for agents. Install it wherever your agent looks for skills, e.g.\n\n" +
			"  mkdir -p .agents/skills/" + skill.Name + " && ward skill > .agents/skills/" + skill.Name + "/SKILL.md\n\n" +
			"The directory name must be '" + skill.Name + "'. Nothing is written by this command.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			_, err := c.OutOrStdout().Write([]byte(skill.Markdown))
			return err
		},
	}
}
