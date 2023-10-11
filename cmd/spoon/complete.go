package main

import (
	"strings"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

func autocompleteAvailable(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	buckets, err := scoop.GetLocalBuckets()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	toComplete = strings.ToLower(toComplete)
	var matches []string

	for _, bucket := range buckets {
		apps, err := bucket.AvailableApps()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		for _, app := range apps {
			if name := app.Name; strings.HasPrefix(name, toComplete) {
				matches = append(matches, name)
			}
		}
	}

	if len(matches) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return matches, cobra.ShellCompDirectiveDefault

}

func autocompleteInstalled(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	toComplete = strings.ToLower(toComplete)
	var matches []string

	apps, err := scoop.GetInstalledApps()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	for _, app := range apps {
		if name := app.Name; strings.HasPrefix(name, toComplete) {
			matches = append(matches, name)
		}
	}

	if len(matches) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return matches, cobra.ShellCompDirectiveDefault
}
