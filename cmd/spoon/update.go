package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Bios-Marcel/spoon/internal/git"
	"github.com/Bios-Marcel/spoon/pkg/scoop"
	gogit "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
)

func fastUpdate(ctx context.Context, repo string) error {
	// Scoop can break repositories when aborting during upgrades. However, we can't use gogit
	// for this, as it will break the repo.
	if err := git.ResetHard(ctx, repo); err != nil {
		return fmt.Errorf("error resetting repo")
	}

	gitRepo, err := gogit.PlainOpen(repo)
	if err != nil {
		return fmt.Errorf("error opening bucket: %w", err)
	}

	workTree, err := gitRepo.Worktree()
	if err != nil {
		return fmt.Errorf("error reading worktree: %w", err)
	}

	if err := workTree.Pull(&gogit.PullOptions{}); err != nil {
		if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
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
			// Whenever we update, whether it is a certain app or the buckets,we
			// always want to make sure everything is in good health and
			// up-to-date. This is helpful, as scoop can break buckets
			// when hitting ctrl-c during an update.

			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				return fmt.Errorf("error getting custom scoop: %w", err)
			}

			buckets, err := defaultScoop.GetLocalBuckets()
			if err != nil {
				return fmt.Errorf("error getting local buckets: %w", err)
			}

			ctx := context.Background()
			var waitgroup sync.WaitGroup
			waitgroup.Add(len(buckets))
			for _, bucket := range buckets {
				bucket := bucket
				go func() {
					defer waitgroup.Done()
					err := fastUpdate(ctx, bucket.Dir())
					if err != nil {
						fmt.Printf("Error updating bucket '%s': %s\n", bucket.Name(), err)
					}
				}()
			}

			waitgroup.Wait()

			// We haven't implemented install yet, therefore we can't
			// handle the actual updating of apps.
			if len(args) > 0 || must(cmd.Flags().GetBool("all")) {
				flags, err := getFlags(cmd, "force", "global", "indepdendent", "no-cache", "skip", "quiet", "all")
				if err != nil {
					return fmt.Errorf("error reading flags: %w", err)
				}

				if code := execScoopCommand("update", append(flags, args...)...); code != 0 {
					os.Exit(code)
				}

				// No need to print status, as everything that can be
				// updated, should be updated at this point in time
				return nil
			}

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
