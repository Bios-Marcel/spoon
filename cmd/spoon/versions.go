package main

import (
	"fmt"
	"math"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

func versionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "versions",
		Short: "Prints all available versions for a given app",
		Args:  cobra.ExactArgs(1),
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
				return fmt.Errorf("app '%s' does not exist", args[0])
			}

			count := must(cmd.Flags().GetInt("count"))
			versions, err := app.AvailableVersionsN(count)
			if err != nil {
				return fmt.Errorf("error retrieving versions: %w", err)
			}

			for _, version := range versions {
				fmt.Println(version)
			}
			return nil
		}),
	}

	cmd.Flags().IntP("count", "c", math.MaxInt32, "defines how many versions you want to fetch (impacts speed)")

	return cmd
}
