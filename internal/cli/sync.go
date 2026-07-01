package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
)

// NewSyncCmd reconciles leaves and store in both directions.
func NewSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Reconcile leaf files and store (both directions), advancing base",
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return fmt.Errorf("store not initialized (run 'ward init')")
			}
			plan, err := e.Plan()
			if err != nil {
				return err
			}
			conflicts, err := e.Apply(plan, engine.Both)
			if err != nil {
				return err
			}
			if conflicts > 0 {
				return fmt.Errorf("%d conflict(s) — run 'ward resolve'", conflicts)
			}
			c.Println("in sync")
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// NewRegisterCmd scans for unregistered keys and adds them.
func NewRegisterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register [path]",
		Short: "Register new leaf files / unregistered keys into the store",
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return fmt.Errorf("store not initialized (run 'ward init')")
			}
			rel := ""
			if len(args) == 1 {
				rel = args[0]
			}
			n, err := e.Register(rel)
			if err != nil {
				return err
			}
			c.Printf("registered %d new key(s)\n", n)
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}
