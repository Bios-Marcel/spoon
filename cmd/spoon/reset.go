package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func resetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "reset",
		Short:             "Reset an app to resolve conflicts",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: autocompleteInstalled,
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			flags, err := getFlags(cmd, "all")
			if err != nil {
				return fmt.Errorf("error getting flags: %w", err)
			}

			os.Exit(execScoopCommand("reset", append(flags, args...)...))
			return nil
		}),
	}

	cmd.Flags().BoolP("all", "a", false, "Reset all apps (alternative to '*')")

	return cmd
}
