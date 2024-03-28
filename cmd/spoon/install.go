package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func installCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "install",
		Short:             "Install a package",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		Run: func(cmd *cobra.Command, args []string) {
			flags, err := getFlags(cmd, "global", "independent", "no-cache", "no-update-scoop", "skip", "arch")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			// Default path, where we can't do our simple optimisation of
			// parallelising install and download, as we only have one package.
			if len(args) == 1 {
				os.Exit(execScoopCommand("install", append(flags, args...)...))
				return
			}

			// FIXME Parallelise.
			os.Exit(execScoopCommand("install", append(flags, args...)...))
		},
	}

	cmd.Flags().BoolP("global", "g", false, "Install an app globally")
	cmd.Flags().BoolP("independent", "i", false, "Don't install dependencies automatically")
	cmd.Flags().BoolP("no-cache", "k", false, "Don't use download cache")
	cmd.Flags().BoolP("no-update-scoop", "u", false, "Don't use scoop before i if it's outdated")
	cmd.Flags().BoolP("skip", "s", false, "Skip hash validation")
	cmd.Flags().BoolP("arch", "a", false, "use specified architecture, if app supports it")

	return cmd
}
