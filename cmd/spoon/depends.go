package main

import (
	"fmt"
	"strings"

	"github.com/Bios-Marcel/spoon/internal/cli"
	"github.com/Bios-Marcel/spoon/pkg/scoop"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/cobra"
)

func dependsCmd() *cobra.Command {
	cmd := &cobra.Command{
		// TODO USage
		Use:   "depends {app}",
		Short: "Show dependency tree or reverse dependency tree of an app",
		Example: cli.FormatUsageExample(
			"spoon depends poetry",
			"spoon depends -r python",
		),
		Aliases:           []string{"depend", "needs", "need"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				return fmt.Errorf("error getting default scoop: %w", err)
			}
			app, err := defaultScoop.GetAvailableApp(args[0])
			if err != nil {
				return fmt.Errorf("error looking up app: %w", err)
			}

			if app == nil {
				return fmt.Errorf("app '%s' doesn't exist", args[0])
			}

			iter := jsoniter.Parse(jsoniter.ConfigFastest, nil, 1024*128)
			if err := app.LoadDetailsWithIter(iter, scoop.DetailFieldDepends); err != nil {
				return fmt.Errorf("error loading app details: %w", err)
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
					return fmt.Errorf("error getting buckets: %w", err)
				}

				var apps []*scoop.App
				for _, bucket := range buckets {
					bucketApps, err := bucket.AvailableApps()
					if err != nil {
						return fmt.Errorf("error getting available apps for bucket: %w", err)
					}

					for _, app := range bucketApps {
						if err := app.LoadDetailsWithIter(iter, scoop.DetailFieldDepends); err != nil {
							return fmt.Errorf("error loading app details: %w", err)
						}
					}

					apps = append(apps, bucketApps...)
				}

				tree := defaultScoop.ReverseDependencyTree(apps, app)
				printDeps(0, tree)
			} else {
				tree, err := defaultScoop.DependencyTree(app)
				if err != nil {
					return fmt.Errorf("error building dependency tree: %w", err)
				}

				printDeps(0, tree)
			}

			return nil
		}),
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
