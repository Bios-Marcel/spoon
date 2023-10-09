package main

import (
	"fmt"
	"os"
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

func searchCmd() *cobra.Command {
	cmd := &cobra.Command{
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

	cmd.Flags().IntP("workers", "w", runtime.NumCPU(), "Sets the maximum amount of workers to do background tasks with")
	cmd.Flags().BoolP("case-insensitive", "i", true, "Defines whether any text matching is case insensitive")

	// FIXME Add flag completion: cmd.RegisterFlagCompletionFunc
	cmd.Flags().StringArrayP("fields", "f", []string{SearchFieldName, SearchFieldBin, SearchFieldDescription}, "Specifies the fields which are searched in. Available: bin, name, description")
	cmd.Flags().StringArrayP("not-fields", "", nil, "Opposite of --fields")
	cmd.MarkFlagsMutuallyExclusive("fields", "not-fields")

	return cmd
}
