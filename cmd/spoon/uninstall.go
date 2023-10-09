package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func uninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall an installed pacakge (Data is kept by default)",
		Aliases: []string{
			"remove",
			"delete",
			"rm",
		},
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteInstalled,
		Run: func(cmd *cobra.Command, args []string) {
			flags, err := getFlags(cmd, "global", "purge")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(execScoopCommand("uninstall", append(flags, args...)...))
		},
	}

	cmd.Flags().BoolP("global", "g", false, "Uninstall a globally installed app")
	cmd.Flags().BoolP("purge", "p", false, "Remove all persistent data")

	return cmd
}
