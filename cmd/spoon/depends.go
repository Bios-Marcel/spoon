package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/cobra"
)

func dependsCmd() *cobra.Command {
	cmd := &cobra.Command{
		// TODO USage
		Use:   "depends {app}",
		Short: "TODO",
		Long:  "TODO",
		Example: examples(
			"TODO",
		),
		Aliases:           []string{"depend"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		Run: func(cmd *cobra.Command, args []string) {
			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				fmt.Println("error getting default scoop:", err)
				os.Exit(1)
			}
			app, err := defaultScoop.GetAvailableApp(args[0])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			iter := jsoniter.Parse(jsoniter.ConfigFastest, nil, 1024*128)
			if err := app.LoadDetailsWithIter(iter, scoop.DetailFieldDepends); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			reverse := must(cmd.Flags().GetBool("reverse"))

			// TODOs
			// 1. Fancy print tree (Ascii tree guides)
			// 2. JSON
			// 3. Multiarg
			// 4. Speed up

			if reverse {
				buckets, err := defaultScoop.GetLocalBuckets()
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

				var apps []*scoop.App
				for _, bucket := range buckets {
					bucketApps, err := bucket.AvailableApps()
					if err != nil {
						fmt.Println(err)
						os.Exit(1)
					}

					for _, app := range bucketApps {
						if err := app.LoadDetailsWithIter(iter, scoop.DetailFieldDepends); err != nil {
							fmt.Println(err)
							os.Exit(1)
						}
					}

					apps = append(apps, bucketApps...)
				}

				tree := defaultScoop.ReverseDependencyTree(apps, app)
				printDeps(0, tree)
			} else {
				tree, err := defaultScoop.DependencyTree(app)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

				printDeps(0, tree)
			}
		},
	}

	cmd.Flags().BoolP("reverse", "r", false, "Reverse the direction we retrieve dependencies")

	return cmd
}

func printDeps(indent int, dependencies *scoop.Dependencies) {
	indentStr := strings.Repeat("    ", indent)
	fmt.Println(indentStr + dependencies.App.Name)
	for _, dependencies := range dependencies.Values {
		printDeps(indent+1, dependencies)
	}
}
