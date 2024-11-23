package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func holdCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "hold",
		Short:              "Hold an app to disable updates",
		Args:               cobra.MinimumNArgs(1),
		ValidArgsFunction:  autocompleteInstalled,
		DisableFlagParsing: true,
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			flags, err := getFlags(cmd, "global")
			if err != nil {
				return fmt.Errorf("error getting flags: %w", err)
			}

			os.Exit(execScoopCommand("hold", append(flags, args...)...))
			return nil
		}),
	}

	cmd.Flags().BoolP("global", "g", false, "Hold a globally installed app")

	return cmd
}
