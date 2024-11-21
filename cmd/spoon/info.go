package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func infoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "info {app}",
		Short:             "Display information about a specific app",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		Run: func(cmd *cobra.Command, args []string) {
			flags, err := getFlags(cmd, "verbose")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(execScoopCommand("info", append(flags, args...)...))
		},
	}

	return cmd
}
