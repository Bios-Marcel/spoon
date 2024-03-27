package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

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
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				return fmt.Errorf("error getting default scoop: %w", err)
			}

			app, err := defaultScoop.GetAvailableApp(args[0])
			if err != nil {
				return fmt.Errorf("error finding app: %w", err)
			}

			if app == nil {
				return fmt.Errorf("the app couldn't be found")
			}

			handle, err := os.Open(app.ManifestPath())
			if err != nil {
				return fmt.Errorf("error loading app data: %w", err)
			}

			_, err = io.Copy(os.Stdout, handle)
			if err != nil {
				return fmt.Errorf("error reading file: %w", err)
			}
			return nil
		}),
	}
}
