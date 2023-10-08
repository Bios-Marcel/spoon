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

var (
	rootCmd = cobra.Command{
		Use: "spoon",
	}

	searchCmd = cobra.Command{
		Use:  "search",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			search := args[0]
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Println(err)
				return
			}
			dirs, err := filepath.Glob(filepath.Join(home, "scoop/buckets/*/bucket"))
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
								syncQueue <- json.Match{
									Description: app.Description,
									Bucket:      job.bucket,
									Name:        job.name,
								}

								return
							}

							if searchBin {
								switch castBin := app.Bin.(type) {
								case string:
									if contains(filepath.Base(castBin), search, caseInsensitive) {
										syncQueue <- json.Match{
											Description: app.Description,
											Bucket:      job.bucket,
											Name:        job.name,
										}

										return
									}
								case []string:
									for _, bin := range castBin {
										if contains(filepath.Base(bin), search, caseInsensitive) {
											syncQueue <- json.Match{
												Description: app.Description,
												Bucket:      job.bucket,
												Name:        job.name,
											}

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
				dirHandle, err := os.Open(dir)
				if err != nil {
					fmt.Println(err)
					return
				}

				entries, err := dirHandle.Readdir(-1)
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

				tbl := table.New("ID", "Description", "Bucket")

				for _, match := range matches {
					desc := match.Description
					if len(desc) > 50 {
						desc = desc[:50]
					}
					tbl.AddRow(match.Name, desc, match.Bucket)
				}

				tbl.Print()
			}
		},
	}
)

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
	outFormat = rootCmd.PersistentFlags().String("out-format", "plain", "TODO")

	searchCmd.Flags().IntP("workers", "w", runtime.NumCPU(), "TODO")
	searchCmd.Flags().BoolP("case-insensitive", "i", true, "TODO")
	searchCmd.Flags().StringArrayP("fields", "f", []string{SearchFieldName, SearchFieldBin, SearchFieldDescription}, "TODO")
	searchCmd.Flags().StringArrayP("not-fields", "", nil, "TODO")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
