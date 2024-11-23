package main

import (
	"os"

	"github.com/spf13/cobra"
)

func listCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed apps",
		Args:  cobra.MaximumNArgs(1),
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			os.Exit(execScoopCommand("list", args...))
			return nil
		}),
	}

	return cmd
}
