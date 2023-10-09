package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func updateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "update",
		Aliases: []string{
			"upgrade",
			"up",
		},
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteInstalled,
		Run: func(cmd *cobra.Command, args []string) {
			flags, err := getFlags(cmd, "force", "global", "indepdendent", "no-cache", "skip", "quiet", "all")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(execScoopCommand("update", append(flags, args...)...))
		},
	}

	cmd.Flags().BoolP("force", "f", false, "Force update even where tehre isn't a newer version")
	cmd.Flags().BoolP("global", "g", false, "Install an app globally")
	cmd.Flags().BoolP("independent", "i", false, "Don't install dependencies automatically")
	cmd.Flags().BoolP("no-cache", "k", false, "Don't use download cache")
	cmd.Flags().BoolP("skip", "s", false, "Skip hash validation")
	cmd.Flags().BoolP("quiet", "q", false, "Hide extraenous messages")
	cmd.Flags().BoolP("all", "a", false, "Update all apps (alternative to '*')")

	return cmd
}
