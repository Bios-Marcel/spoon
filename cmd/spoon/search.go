package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
)

type matches []match

type match struct {
	Description string `json:"description"`
	Name        string `json:"name"`
	Bucket      string `json:"bucket"`
	Version     string `json:"version"`
}

type searchJob struct {
	bucket string
	app    scoop.App
}

type SearchField string

const (
	SearchFieldName        = "name"
	SearchFieldBin         = "bin"
	SearchFieldDescription = "description"
)

var allSearchFields = []string{SearchFieldName, SearchFieldBin, SearchFieldDescription}

func searchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "search",
		Short:   "Find a scoop package by search query.",
		Aliases: []string{"find", "s"},
		Example: "search git",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			search := args[0]

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

			searchFields, err := cmd.Flags().GetStringSlice("fields")
			if err != nil {
				fmt.Println(err)
				return
			}

			if dontSearchFields, err := cmd.Flags().GetStringSlice("not-fields"); err != nil {
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

			queue := make(chan searchJob)
			var wg sync.WaitGroup

			syncQueue := make(chan match, searchWorkers)
			match := func(job searchJob, app scoop.App) {
				syncQueue <- match{
					Description: app.Description,
					Version:     app.Version,
					Bucket:      job.bucket,
					Name:        app.Name,
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

							if err := job.app.LoadDetails(); err != nil {
								fmt.Println("Error loading app metadata")
								os.Exit(1)
							}

							app := job.app
							if (searchName && equals(app.Name, search, caseInsensitive)) ||
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

			buckets, err := scoop.GetLocalBuckets()
			if err != nil {
				fmt.Println("error getting buckets:", err)
				os.Exit(1)
			}
			for _, bucket := range buckets {
				apps, err := bucket.AvailableApps()
				if err != nil {
					fmt.Println("error getting bucket manifests:", err)
					os.Exit(1)
				}

				wg.Add(len(apps))
				go func(bucketName string) {
					for _, app := range apps {
						queue <- searchJob{
							app:    app,
							bucket: bucketName,
						}
					}
				}(bucket.Name())
			}

			var matchList matches
			go func() {
				for {
					matchList = append(matchList, <-syncQueue)
					wg.Done()
				}
			}()

			wg.Wait()

			switch *outFormat {
			case "json":
				if err := json.NewEncoder(os.Stdout).Encode(matchList); err != nil {
					fmt.Println(err)
					return
				}
			case "plain":
				sort.Slice(matchList, func(i, j int) bool {
					a, b := matchList[i], matchList[j]
					return a.Name < b.Name
				})

				tbl := table.New("Name", "Version", "Bucket", "Description")

				for _, match := range matchList {
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

	cmd.Flags().IntP("workers", "w", runtime.NumCPU(), "Sets the maximum amount of workers to do background tasks with")
	cmd.Flags().BoolP("case-insensitive", "i", true, "Defines whether any text matching is case insensitive")

	cmd.Flags().StringSliceP("fields", "f", allSearchFields, "Specifies the fields which are searched in. Available: bin, name, description")
	cmd.Flags().StringSliceP("not-fields", "", nil, "Opposite of --fields")
	cmd.RegisterFlagCompletionFunc("fields", autocompleteSearchFieldFlag)
	cmd.RegisterFlagCompletionFunc("not-fields", autocompleteSearchFieldFlag)
	cmd.MarkFlagsMutuallyExclusive("fields", "not-fields")

	return cmd
}

// autocompleteSearchFieldFlag will autocomplete single search fields. This does
// not allow passing things such as "bin,desc<Complete>". For some reason this
// does not work.
func autocompleteSearchFieldFlag(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if toComplete == "" {
		return allSearchFields, cobra.ShellCompDirectiveNoFileComp
	}

	var leftoverFields []string
	for _, field := range allSearchFields {
		if strings.HasPrefix(field, toComplete) {
			leftoverFields = append(leftoverFields, field)
		}
	}

	if len(leftoverFields) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return leftoverFields, cobra.ShellCompDirectiveNoFileComp
}
