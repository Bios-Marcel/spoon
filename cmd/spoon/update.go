package main

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	git "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
)

func fastUpdate(repo string) error {
	gitRepo, err := git.PlainOpen(repo)
	if err != nil {
		return fmt.Errorf("error opening bucket: %w", err)
	}

	workTree, err := gitRepo.Worktree()
	if err != nil {
		return fmt.Errorf("error reading worktree: %w", err)
	}

	if err := workTree.Pull(&git.PullOptions{}); err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil
		}
		return fmt.Errorf("error pulling updates: %w", err)
	}

	return nil
}

func updateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update an installed package",
		Aliases: []string{
			"upgrade",
			"up",
			"refresh",
		},
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: autocompleteInstalled,
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			// If we have no args, it means update the buckets, instead of apps.
			// We do this natively, as it's much faster and a rather easy task.
			// However, we haven't implemented install yet, therefore we can't
			// handle the actual updating of apps.
			if len(args) > 0 {
				flags, err := getFlags(cmd, "force", "global", "indepdendent", "no-cache", "skip", "quiet", "all")
				if err != nil {
					return fmt.Errorf("error reading flags: %w", err)
				}

				os.Exit(execScoopCommand("update", append(flags, args...)...))
				return nil
			}

			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				return fmt.Errorf("error getting custom scoop: %w", err)
			}

			buckets, err := defaultScoop.GetLocalBuckets()
			if err != nil {
				return fmt.Errorf("error getting local buckets: %w", err)
			}

			var waitgroup sync.WaitGroup
			waitgroup.Add(len(buckets))
			for _, bucket := range buckets {
				bucket := bucket
				go func() {
					defer waitgroup.Done()
					err := fastUpdate(bucket.Dir())
					if err != nil {
						fmt.Printf("Error updating bucket '%s': %s\n", bucket.Name(), err)
					}
				}()
			}

			waitgroup.Wait()

			// Since you usually want to know whether anything has changed, we
			// can just run this right away.
			return status(defaultScoop)
		}),
	}

	cmd.Flags().BoolP("force", "f", false, "Force update even where there isn't a newer version")
	cmd.Flags().BoolP("global", "g", false, "Install an app globally")
	cmd.Flags().BoolP("independent", "i", false, "Don't install dependencies automatically")
	cmd.Flags().BoolP("no-cache", "k", false, "Don't use download cache")
	cmd.Flags().BoolP("skip", "s", false, "Skip hash validation")
	cmd.Flags().BoolP("quiet", "q", false, "Hide extraneous messages")
	cmd.Flags().BoolP("all", "a", false, "Update all apps (alternative to '*')")

	return cmd
}
