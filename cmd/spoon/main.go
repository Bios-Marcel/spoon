package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func execScoopCommand(command string, args ...string) int {
	cmd := exec.Command("scoop", append([]string{command}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return 1
	}

	return 0
}

func getFlags(cmd *cobra.Command, flags ...string) ([]string, error) {
	var outFlags []string
	for _, name := range flags {
		if !cmd.Flags().Changed(name) {
			continue
		}

		flag := cmd.Flags().Lookup(name)
		switch flag.Value.Type() {
		case "bool":
			outFlags = append(outFlags, "--"+name)
		default:
			outFlags = append(outFlags, "--"+name, flag.Value.String())
		}
	}

	return outFlags, nil
}

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

func getBucketDirs() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error getting home directory: %w", err)
	}

	return filepath.Glob(filepath.Join(home, "scoop/buckets/*/bucket"))
}

func getInstalledManifests() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error getting home directory: %w", err)
	}

	return filepath.Glob(filepath.Join(home, "scoop/apps/*/current/manifest.json"))
}

func getDirEntries(dir string) ([]fs.FileInfo, error) {
	dirHandle, err := os.Open(dir)
	if err != nil {
		return nil, err
	}

	return dirHandle.Readdir(-1)
}

// equals checks for string equality, optionally ignoring casing. The value `b`
// is expected to be lowered already, if `ci` has been set.
func equals(a, b string, ci bool) bool {
	if ci {
		return strings.EqualFold(a, b)
	}

	return a == b
}

// equals checks whether `whole` contains substring `find`, optionally ignoring
// casing. The value `find` is expected to be lowered already, if `ci` has been
// set.
func contains(whole, find string, ci bool) bool {
	if ci {
		// FIXME Depending on casing rules, this might not hold true.
		if len(find) > len(whole) {
			return false
		}

		return strings.Contains(strings.ToLower(whole), find)
	}

	return strings.Contains(whole, find)
}

type job struct {
	bucket string
	path   string
	name   string
}

var (
	outFormat *string
)

type SearchField string

const (
	SearchFieldName        = "name"
	SearchFieldBin         = "bin"
	SearchFieldDescription = "description"
)

func main() {
	rootCmd := cobra.Command{
		Use:   "spoon",
		Short: "Wrapper around scoop, that offers the same functionality, but better.",
	}

	rootCmd.AddCommand(searchCmd())
	rootCmd.AddCommand(installCmd())
	rootCmd.AddCommand(uninstallCmd())
	rootCmd.AddCommand(updateCmd())

	outFormat = rootCmd.PersistentFlags().String("out-format", "plain", "Specifies the output format to use for any data printed")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
