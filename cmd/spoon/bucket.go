package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Bios-Marcel/spoon/internal/cli"
	"github.com/Bios-Marcel/spoon/internal/collections"
	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

func bucketCmd() *cobra.Command {
	bucketRoot := &cobra.Command{
		Use:   "bucket",
		Short: "Allows managing installed buckets",
		Long:  "Allows adding and removing buckets to the scoop install. Adding a bucket makes the contained apps available for installation.",
	}

	bucketRoot.AddCommand(
		&cobra.Command{
			Use: "add { bucket | name url }",
			Aliases: []string{
				"install",
			},
			Short: "Adds a bucket to scoop",
			Long: strings.TrimSpace(`
Add a bucket to scoop. This allows you to install apps from that bucket.

This command accepts one or two arguments. Either a known bucket (see spoon bucket known) or "bucketname" "url".`),
			Example: cli.FormatUsageExample(
				"spoon bucket add games",
				"spoon bucket custom https://github.com/user/repo.git",
			),
			// Either a "known bucket" or "bucket name" "url".
			Args: cobra.RangeArgs(1, 2),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				// Since buckets only supports a single bucket at once, we
				// don't need comp after a name has been added.
				if len(args) != 0 {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}

				defaultScoop, err := scoop.NewScoop()
				if err != nil {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}

				knownBuckets, err := defaultScoop.GetKnownBuckets()
				if err != nil {
					return nil, cobra.ShellCompDirectiveDefault
				}

				return knownBucketNames(knownBuckets), cobra.ShellCompDirectiveDefault
			},
			Run: func(cmd *cobra.Command, args []string) {
				os.Exit(execScoopCommand("bucket add", args...))
			},
		},
		&cobra.Command{
			Use: "rm",
			Aliases: []string{
				"remove",
				"delete",
				"uninstall",
			},
			Short: "Removes bucket(s) from scoop",
			Example: cli.FormatUsageExample(
				"spoon bucket rm games",
				"spoon bucket rm games extras java",
			),
			Args: cobra.MinimumNArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				defaultScoop, err := scoop.NewScoop()
				if err != nil {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}
				buckets, err := defaultScoop.GetLocalBuckets()
				if err != nil {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}

				var bucketNames []string
			BUCKET_LOOP:
				for _, bucket := range buckets {
					for _, arg := range args {
						if bucket.Name() == arg {
							continue BUCKET_LOOP
						}
					}
					bucketNames = append(bucketNames, bucket.Name())
				}

				return bucketNames, cobra.ShellCompDirectiveDefault
			},
			Run: func(cmd *cobra.Command, args []string) {
				defaultScoop, err := scoop.NewScoop()
				if err != nil {
					fmt.Println("error getting default scoop:", err)
					os.Exit(1)
				}
				buckets, err := defaultScoop.GetLocalBuckets()
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

				var bucketsToDelete []*scoop.Bucket
			BUCKET_LOOP:
				for _, bucket := range buckets {
					for _, arg := range args {
						if bucket.Name() == arg {
							bucketsToDelete = append(bucketsToDelete, bucket)
							continue BUCKET_LOOP
						}
					}
				}

				var failed bool
				for _, bucket := range bucketsToDelete {
					fmt.Printf("Removing bucket '%s'...\n", bucket.Name())
					if err := bucket.Remove(); err != nil {
						fmt.Printf("Failed to remove bucket '%s': %s\n", bucket.Name(), err)
						failed = true
					}
				}

				if failed {
					os.Exit(1)
				}
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "Lists all added buckets",
			Run: func(cmd *cobra.Command, args []string) {
				os.Exit(execScoopCommand("bucket list"))
			},
		},
	)

	knownCmd := &cobra.Command{
		Use:   "known",
		Short: "Lists all known buckets",
		Run: func(cmd *cobra.Command, args []string) {
			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				fmt.Println("error getting default scoop:", err)
				os.Exit(1)
			}

			knownBuckets, err := defaultScoop.GetKnownBuckets()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			format, err := cmd.Flags().GetString("out-format")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			switch format {
			case "plain":
				for _, bucket := range knownBuckets {
					fmt.Println(bucket.Name)
				}
			case "json":
				bucketNames := knownBucketNames(knownBuckets)
				if err := json.NewEncoder(os.Stdout).Encode(bucketNames); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			}
		},
	}
	knownCmd.Flags().String("out-format", "plain", "Specifies the output format to use for any data printed")

	bucketRoot.AddCommand(knownCmd)
	return bucketRoot
}

func knownBucketNames(buckets []scoop.KnownBucket) []string {
	return collections.FlattenSlice(buckets, func(bucket scoop.KnownBucket) string {
		return bucket.Name
	})
}
