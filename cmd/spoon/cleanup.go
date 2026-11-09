package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

func cleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "cleanup <app>...",
		Short:             "Cleanup apps by removing old versions",
		ValidArgsFunction: autocompleteInstalled,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !must(cmd.Flags().GetBool("all")) {
				return errors.New("<app> missing")
			}
			return nil
		},
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			cleanAll := must(cmd.Flags().GetBool("all")) || containsArg(args, "*")
			global := must(cmd.Flags().GetBool("global"))
			removeCache := must(cmd.Flags().GetBool("cache"))

			if global {
				flags, err := getFlags(cmd, "all", "global", "cache")
				if err != nil {
					return fmt.Errorf("error getting flags: %w", err)
				}
				os.Exit(execScoopCommand("cleanup", append(flags, args...)...))
				return nil
			}

			localScoop, err := scoop.NewScoop()
			if err != nil {
				return err
			}

			var targets []string
			if cleanAll {
				targets, err = localScoop.InstalledAppNames()
				if err != nil {
					return err
				}
			} else {
				targets, err = explicitCleanupTargets(args, localScoop)
				if err != nil {
					return err
				}
			}

			var errs []error
			for _, app := range targets {
				removing := false
				result, err := localScoop.Cleanup(app, scoop.CleanupOptions{
					RemoveCache: removeCache,
					CacheDir:    localScoop.CacheDir(),
					BeforeRemoveVersion: func(version string) {
						if !removing {
							fmt.Printf("Removing %s:", app)
							removing = true
						}
						fmt.Printf(" %s", version)
					},
				})
				if removing {
					fmt.Println()
				} else if len(result.RemovedVersions) == 0 {
					if !cleanAll {
						fmt.Printf("%s is already clean\n", app)
					}
				}
				if err != nil {
					errs = append(errs, fmt.Errorf("error cleaning up %q: %w", app, err))
				}
			}

			if removeCache {
				if err := scoop.CleanupIncompleteDownloads(localScoop.CacheDir()); err != nil {
					errs = append(errs, err)
				}
			}

			if cleanAll && len(errs) == 0 {
				fmt.Println("Everything is shiny now!")
			}
			return errors.Join(errs...)
		}),
	}

	cmd.Flags().BoolP("all", "a", false, "Cleanup all apps (alternative to '*')")
	cmd.Flags().BoolP("global", "g", false, "Cleanup globally installed apps")
	cmd.Flags().BoolP("cache", "k", false, "Remove outdated download cache")

	return cmd
}

func containsArg(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func explicitCleanupTargets(
	args []string,
	localScoop *scoop.Scoop,
) ([]string, error) {
	seen := make(map[string]struct{}, len(args))
	targets := make([]string, 0, len(args))
	for _, arg := range args {
		_, app, _ := scoop.ParseAppIdentifier(arg)
		if err := validateCleanupAppName(app); err != nil {
			return nil, err
		}
		if strings.EqualFold(app, "scoop") {
			continue
		}
		key := strings.ToLower(app)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}

		installed, err := isAppDirectory(localScoop, app)
		if err != nil {
			return nil, err
		}
		if installed {
			targets = append(targets, app)
			continue
		}
		fmt.Printf("ERROR '%s' isn't installed.\n", app)
	}
	return targets, nil
}

func validateCleanupAppName(app string) error {
	if app == "" || app == "." || app == ".." || filepath.Base(app) != app ||
		strings.ContainsAny(app, `/\\`) {
		return fmt.Errorf("invalid app name %q", app)
	}
	return nil
}

func isAppDirectory(scoop *scoop.Scoop, app string) (bool, error) {
	info, err := os.Stat(filepath.Join(scoop.AppDir(), app))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("error checking installation of %q: %w", app, err)
	}
	return info.IsDir(), nil
}
