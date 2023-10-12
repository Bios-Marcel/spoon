package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

func bucketCmd() *cobra.Command {
	bucketRoot := &cobra.Command{
		Use: "bucket",
	}

	bucketRoot.AddCommand(
		&cobra.Command{
			Use: "add",
			// Either a "known bucket" or "bucket name" "url".
			Args: cobra.RangeArgs(1, 2),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				// Since buckets only supports a single bucket at once, we
				// don't need comp after a name has been added.
				if len(args) != 0 {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}

				knownBuckets, err := getKnownBucketsFlat()
				if err != nil {
					return nil, cobra.ShellCompDirectiveDefault
				}

				return knownBuckets, cobra.ShellCompDirectiveDefault
			},
			Run: func(cmd *cobra.Command, args []string) {
				os.Exit(execScoopCommand("bucket add", args...))
			},
		},
		&cobra.Command{
			Use:  "rm",
			Args: cobra.MinimumNArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				buckets, err := scoop.GetLocalBuckets()
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
				// FIXME Implement ourselves
				for _, bucketName := range args {
					execScoopCommand("bucket rm", bucketName)
				}
			},
		},
		&cobra.Command{
			Use: "list",
			Run: func(cmd *cobra.Command, args []string) {
				os.Exit(execScoopCommand("bucket list"))
			},
		},
		&cobra.Command{
			Use: "known",
			Run: func(cmd *cobra.Command, args []string) {
				knownBuckets, err := getKnownBucketsFlat()
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

				format, err := cmd.InheritedFlags().GetString("out-format")
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

				switch format {
				case "plain":
					for _, bucketName := range knownBuckets {
						fmt.Println(bucketName)
					}
				case "json":
					if err := json.NewEncoder(os.Stdout).Encode(knownBuckets); err != nil {
						fmt.Println(err)
						os.Exit(1)
					}
				}
			},
		},
	)

	return bucketRoot
}

func getKnownBucketsFlat() ([]string, error) {
	knownBuckets, err := scoop.GetKnownBuckets()
	if err != nil {
		return nil, fmt.Errorf("error getting known buckets: %w", err)
	}

	var flattened []string
	for bucketName := range knownBuckets {
		flattened = append(flattened, bucketName)
	}

	return flattened, nil
}
