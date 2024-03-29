package main

import (
	"fmt"
	"strings"

	"github.com/Bios-Marcel/spoon/internal/cli"
	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func status(scoop *scoop.Scoop) error {
	apps, err := scoop.GetOutdatedApps()
	if err != nil {
		return fmt.Errorf("error getting installed apps: %w", err)
	}

	tbl, _, _ := cli.CreateTable("Name", "Installed Version", "Latest Version", "Missing Dependencies", "Info")

	for _, app := range apps {
		var info []string
		if app.Bucket == nil {
			info = append(info, "Not in any bucket")
		}
		if app.ManifestDeleted {
			info = append(info, "Manifest removed")
		}
		if app.Hold {
			info = append(info, "Held package")
		}
		tbl.AddRow(app.Name, app.Version, app.LatestVersion, "", strings.Join(info, ","))
	}

	fmt.Print("\n")
	tbl.Print()
	fmt.Print("\n")
	return nil
}

func statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display available updates for installed apps",
		Args:  cobra.NoArgs,
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				return fmt.Errorf("error getting default scoop: %w", err)
			}

			local := must(cmd.Flags().GetBool("local"))
			if local {
				fmt.Println(color.YellowString("NOTE `--local` flag isn't supported anymore."))
				fmt.Println(color.YellowString("Run `spoon update` instead, which also runs `spoon status`."))
			}

			return status(defaultScoop)
		}),
	}

	cmd.Flags().BoolP("local", "l", false, "Disable remote fetching/checking for updates")

	return cmd
}
