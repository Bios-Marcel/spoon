package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func main() {
	// This seems to provide no value whatsoever, it seemingly doesn't even do
	// what's documented. All it does, is take time.
	cobra.MousetrapHelpText = ""

	rootCmd := cobra.Command{
		Use:   "spoon",
		Short: "Wrapper around scoop, that offers the same functionality, but better.",
		// By default, subcommand aliases aren't autocompleted.
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			var aliases []string
			for _, subCmd := range cmd.Commands() {
				aliases = append(aliases, subCmd.Aliases...)
			}
			return aliases, cobra.ShellCompDirectiveNoFileComp
		},
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
	}

	rootCmd.AddCommand(searchCmd())
	rootCmd.AddCommand(installCmd())
	rootCmd.AddCommand(uninstallCmd())
	rootCmd.AddCommand(updateCmd())
	rootCmd.AddCommand(bucketCmd())
	rootCmd.AddCommand(catCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(infoCmd())

	if err := rootCmd.Execute(); err != nil {
		if strings.HasPrefix(err.Error(), "unknown command") {
			fmt.Println("Delegating to scoop ...")
			execScoopCommand(os.Args[1], os.Args[2:]...)
		} else {
			fmt.Println("error:", err)
			os.Exit(1)
		}
	}
}
