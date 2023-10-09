package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Bios-Marcel/spoon/internal/json"
	"github.com/mailru/easyjson"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
)

func execScoopCommand(command string, args ...string) int {
	cmd := exec.Command("scoop", append([]string{command}, args...)...)
	fmt.Println(cmd.String())
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

var (
	rootCmd = cobra.Command{
		Use: "spoon",
	}

	uninstallCmd = cobra.Command{
		Use: "uninstall",
		Aliases: []string{
			"remove",
			"delete",
			"rm",
		},
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteInstalled,
		Run: func(cmd *cobra.Command, args []string) {
			flags, err := getFlags(cmd, "global", "purge")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(execScoopCommand("uninstall", append(flags, args...)...))
		},
	}

	updateCmd = cobra.Command{
		Use: "update",
		Aliases: []string{
			"upgrade",
			"up",
		},
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteInstalled,
		Run: func(cmd *cobra.Command, args []string) {
			flags, err := getFlags(cmd, "force", "global", "indepdendent", "no-cache", "skip", "quiet", "all")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(execScoopCommand("update", append(flags, args...)...))
		},
	}
	installCmd = cobra.Command{
		Use:               "install",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		Run: func(cmd *cobra.Command, args []string) {
			flags, err := getFlags(cmd, "global", "independent", "no-cache", "no-update-scoop", "skip", "arch")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(execScoopCommand("install", append(flags, args...)...))
		},
	}

	searchCmd = cobra.Command{
		Use:     "search",
		Short:   "Find a scoop package by search query.",
		Aliases: []string{"find", "s"},
		Example: "search git",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			search := args[0]
			dirs, err := getBucketDirs()
			if err != nil {
				fmt.Println(err)
				return
			}

			searchWorkers, err := cmd.Flags().GetInt("workers")
			if err != nil {
				fmt.Println(err)
				return
			}

			caseInsensitive, err := cmd.Flags().GetBool("case-insensitive")
			if err != nil {
				fmt.Println(err)
				return
			}
			if caseInsensitive {
				// FIXME Case folding with any language?
				search = strings.ToLower(search)
			}

			searchFields, err := cmd.Flags().GetStringArray("fields")
			if err != nil {
				fmt.Println(err)
				return
			}

			if dontSearchFields, err := cmd.Flags().GetStringArray("not-fields"); err != nil {
				fmt.Println(err)
				return
			} else {
				slices.DeleteFunc(searchFields, func(s string) bool {
					return slices.Contains(dontSearchFields, s)
				})
			}

			searchName := slices.Contains(searchFields, SearchFieldName)
			searchBin := slices.Contains(searchFields, SearchFieldBin)
			searchDescription := slices.Contains(searchFields, SearchFieldDescription)

			queue := make(chan job)
			var wg sync.WaitGroup

			syncQueue := make(chan json.Match, searchWorkers)
			match := func(job job, app json.App) {
				syncQueue <- json.Match{
					Description: app.Description,
					Version:     app.Version,
					Bucket:      job.bucket,
					Name:        job.name,
				}
			}

			for i := 0; i < searchWorkers; i++ {
				go func() {
					for {
						job := <-queue
						func() {
							// Prevent deadlocks
							defer func() {
								if err := recover(); err != nil {
									wg.Done()
								}
							}()

							// We intentionally keep handles closed, as we
							// close them anyway once the process dies.
							file, err := os.Open(job.path)
							if err != nil {
								fmt.Println(err)
								os.Exit(1)
							}

							var app json.App
							if err := easyjson.UnmarshalFromReader(file, &app); err != nil {
								fmt.Println(err)
								os.Exit(1)
							}

							if (searchName && equals(job.name, search, caseInsensitive)) ||
								(searchDescription && contains(app.Description, search, caseInsensitive)) {
								match(job, app)
								return
							}

							if searchBin {
								switch castBin := app.Bin.(type) {
								case string:
									if contains(filepath.Base(castBin), search, caseInsensitive) {
										match(job, app)
										return
									}
								case []string:
									for _, bin := range castBin {
										if contains(filepath.Base(bin), search, caseInsensitive) {
											match(job, app)
											return
										}
									}
								}
							}

							wg.Done()
						}()
					}
				}()
			}

			for _, dir := range dirs {
				bucket := filepath.Base(filepath.Dir(dir))
				entries, err := getDirEntries(dir)
				if err != nil {
					fmt.Println(err)
					return
				}

				wg.Add(len(entries))
				go func(dir string) {
					for _, entry := range entries {
						name := entry.Name()
						queue <- job{
							path:   filepath.Join(dir, name),
							bucket: bucket,
							// Cut off .json
							name: name[:len(name)-5],
						}
					}
				}(dir)
			}

			var matches json.Matches
			go func() {
				for {
					matches = append(matches, <-syncQueue)
					wg.Done()
				}
			}()

			wg.Wait()

			switch *outFormat {
			case "json":
				if _, err := easyjson.MarshalToWriter(matches, os.Stdout); err != nil {
					fmt.Println(err)
					return
				}
			case "plain":
				sort.Slice(matches, func(i, j int) bool {
					a, b := matches[i], matches[j]
					return a.Name < b.Name
				})

				tbl := table.New("Name", "Version", "Bucket", "Description")

				for _, match := range matches {
					desc := match.Description
					if len(desc) > 50 {
						desc = desc[:47] + "..."
					}
					tbl.AddRow(match.Name, match.Version, match.Bucket, desc)
				}

				tbl.Print()
			}
		},
	}
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

func init() {
	rootCmd.AddCommand(&searchCmd)
	rootCmd.AddCommand(&installCmd)
	rootCmd.AddCommand(&uninstallCmd)
	rootCmd.AddCommand(&updateCmd)

	uninstallCmd.Flags().BoolP("global", "g", false, "Uninstall a globally installed app")
	uninstallCmd.Flags().BoolP("purge", "p", false, "Remove all persistent data")

	installCmd.Flags().BoolP("global", "g", false, "Install an app globally")
	installCmd.Flags().BoolP("independent", "i", false, "Don't install dependencies automatically")
	installCmd.Flags().BoolP("no-cache", "k", false, "Don't use download cache")
	installCmd.Flags().BoolP("no-update-scoop", "u", false, "Don't use scoop before i if it's outdated")
	installCmd.Flags().BoolP("skip", "s", false, "Skip hash validation")
	installCmd.Flags().BoolP("arch", "a", false, "use specified architechture, if app supports it")

	updateCmd.Flags().BoolP("force", "f", false, "Force update even where tehre isn't a newer version")
	updateCmd.Flags().BoolP("global", "g", false, "Install an app globally")
	updateCmd.Flags().BoolP("independent", "i", false, "Don't install dependencies automatically")
	updateCmd.Flags().BoolP("no-cache", "k", false, "Don't use download cache")
	updateCmd.Flags().BoolP("skip", "s", false, "Skip hash validation")
	updateCmd.Flags().BoolP("quiet", "q", false, "Hide extraenous messages")
	updateCmd.Flags().BoolP("all", "a", false, "Update all apps (alternative to '*')")

	outFormat = rootCmd.PersistentFlags().String("out-format", "plain", "Specifies the output format to use for any data printed")

	searchCmd.Flags().IntP("workers", "w", runtime.NumCPU(), "Sets the maximum amount of workers to do background tasks with")
	searchCmd.Flags().BoolP("case-insensitive", "i", true, "Defines whether any text matching is case insensitive")

	// FIXME Add flag completion: searchCmd.RegisterFlagCompletionFunc
	searchCmd.Flags().StringArrayP("fields", "f", []string{SearchFieldName, SearchFieldBin, SearchFieldDescription}, "Specifies the fields which are searched in. Available: bin, name, description")
	searchCmd.Flags().StringArrayP("not-fields", "", nil, "Opposite of --fields")
	searchCmd.MarkFlagsMutuallyExclusive("fields", "not-fields")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
