package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

func downloadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "download",
		Short:             "Download all files required for a package",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			arch := scoop.ArchitectureKey(must(cmd.Flags().GetString("arch")))
			force := must(cmd.Flags().GetBool("force"))
			noHashCheck := must(cmd.Flags().GetBool("no-hash-check"))

			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				return fmt.Errorf("error retrieving scoop instance: %w", err)
			}

			for _, arg := range args {
				app, err := defaultScoop.FindAvailableApp(arg)
				if err != nil {
					return fmt.Errorf("error looking up app: %w", err)
				}
				if app == nil {
					return fmt.Errorf("app '%s' not found", arg)
				}

				if err := app.LoadDetails(
					scoop.DetailFieldArchitecture,
					scoop.DetailFieldUrl,
					scoop.DetailFieldHash,
				); err != nil {
					return fmt.Errorf("error loading app details: %w", err)
				}

				resolvedApp := app.ForArch(arch)
				resultChan, err := resolvedApp.Download(
					defaultScoop.CacheDir(), arch, !noHashCheck, force,
				)
				if err != nil {
					return err
				}

				for result := range resultChan {
					switch result := result.(type) {
					case *scoop.CacheHit:
						name := filepath.Base(result.Downloadable.URL)
						fmt.Printf("Cache hit for '%s'\n", name)
					case *scoop.FinishedDownload:
						name := filepath.Base(result.Downloadable.URL)
						fmt.Printf("Downloaded '%s'\n", name)
					case error:
						var checksumErr *scoop.ChecksumMismatchError
						if errors.As(result, &checksumErr) {
							fmt.Printf(
								"Checksum mismatch:\n\rFile: '%s'\n\tExpected: '%s'\n\tActual: '%s'\n",
								checksumErr.File,
								checksumErr.Expected,
								checksumErr.Actual,
							)

							// FIXME Find a better way to do this via
							// returnvalue?
							os.Exit(1)
						}
						if result != nil {
							return result
						}
					}
				}
			}

			return nil
		}),
	}

	cmd.Flags().BoolP("force", "f", false, "Force download (overwrite cache)")
	// FIXME No shorthand for now, since --h is help and seems to clash.
	cmd.Flags().Bool("no-hash-check", false, "Skip hash verification (use with caution!)")
	// We default to our system architecture here. If scoop encounters an
	// unsupported arch, it is ignored. We'll do the same.
	cmd.Flags().StringP("arch", "a", string(SystemArchitecture),
		"use specified architecture, if app supports it")
	cmd.RegisterFlagCompletionFunc("arch", cobra.FixedCompletions(
		[]string{
			string(scoop.ArchitectureKey32Bit),
			string(scoop.ArchitectureKey64Bit),
			string(scoop.ArchitectureKeyARM64),
		},
		cobra.ShellCompDirectiveDefault))

	return cmd
}
