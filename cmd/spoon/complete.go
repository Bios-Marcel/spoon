package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func autocompleteAvailable(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	buckets, err := getBucketDirs()
	if err != nil {
		fmt.Println(err)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	toComplete = strings.ToLower(toComplete)
	var matches []string
	for _, bucket := range buckets {
		manifests, err := getDirEntries(bucket)
		if err != nil {
			fmt.Println(err)
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		for _, manifest := range manifests {
			name := strings.TrimSuffix(manifest.Name(), ".json")
			if strings.HasPrefix(name, toComplete) {
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

	manifests, err := getInstalledManifests()
	if err != nil {
		fmt.Println(err)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	for _, manifest := range manifests {
		name := filepath.Base(filepath.Dir(filepath.Dir(manifest)))
		if strings.HasPrefix(name, toComplete) {
			matches = append(matches, name)
		}
	}

	if len(matches) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return matches, cobra.ShellCompDirectiveDefault
}
