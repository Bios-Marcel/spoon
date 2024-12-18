package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Bios-Marcel/spoon/internal/cli"
	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

func catCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cat {app}",
		Short: "Print JSON manifest of an available app",
		Long:  "Print JSON manifest of an available app. Optionally this command accepts a URL to a manifest file.",
		Example: cli.FormatUsageExample(
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

			var app *scoop.App
			if !must(cmd.Flags().GetBool("installed")) {
				app, err = defaultScoop.FindAvailableApp(args[0])
				if err != nil {
					return fmt.Errorf("error finding app: %w", err)
				}
			}

			if app == nil {
				installedApp, err := defaultScoop.FindInstalledApp(args[0])
				if err != nil {
					return fmt.Errorf("error finding app: %w", err)
				}
				if installedApp == nil {
					return fmt.Errorf("the app couldn't be found")
				}
				app = installedApp.App
			}

			var reader io.Reader
			_, _, version := scoop.ParseAppIdentifier(args[0])
			if version != "" {
				reader, err = app.ManifestForVersion(version)
			} else {
				fileReader, tempErr := os.Open(app.ManifestPath())
				if fileReader != nil {
					defer fileReader.Close()
					reader = fileReader
				} else {
					err = tempErr
				}
			}

			if err != nil {
				return fmt.Errorf("error loading manifest: %w", err)
			}

			if reader == nil {
				return fmt.Errorf("the app isn't offered in the desired version")
			}

			_, err = io.Copy(os.Stdout, reader)
			if err != nil {
				return fmt.Errorf("error reading manifest: %w", err)
			}
			return nil
		}),
	}

	cmd.Flags().BoolP("installed", "i", false, "Show manifest of installed app")

	return cmd
}
