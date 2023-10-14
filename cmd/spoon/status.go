package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display available updates for installed apps",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			flags, err := getFlags(cmd, "local")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(execScoopCommand("status", append(flags, args...)...))
		},
	}

	cmd.Flags().BoolP("local", "l", false, "Disable remote fetching/checking for updates")

	return cmd
}
