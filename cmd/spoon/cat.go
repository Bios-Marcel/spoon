package main

import "github.com/spf13/cobra"

func catCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cat {app}",
		Short: "Print JSON manifest of an available app",
		Long:  "Print JSON manifest of an available app. Optionally this command accepts a URL to a manifest file.",
		Example: examples(
			"spoon cat 7zip",
			"spoon cat https://raw.githubusercontent.com/ScoopInstaller/Main/master/bucket/git.json",
		),
		Aliases:           []string{"manifest"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		Run: func(cmd *cobra.Command, args []string) {
			execScoopCommand("cat", args...)
		},
	}
}
