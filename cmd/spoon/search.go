package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	_ "runtime/pprof"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	jsoniter "github.com/json-iterator/go"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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

type SortField string

const (
	SortFieldName   = "name"
	SortFieldBucket = "bucket"
)

var allSearchFields = []string{SearchFieldName, SearchFieldBin, SearchFieldDescription}

func searchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "search",
		Short:   "Find a scoop package by search query.",
		Aliases: []string{"find"},
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
				searchFields = slices.DeleteFunc(searchFields, func(s string) bool {
					return slices.Contains(dontSearchFields, s)
				})
			}

			searchName := slices.Contains(searchFields, SearchFieldName)
			searchBin := slices.Contains(searchFields, SearchFieldBin)
			searchDescription := slices.Contains(searchFields, SearchFieldDescription)

			buckets, err := scoop.GetLocalBuckets()
			if err != nil {
				fmt.Println("error getting buckets:", err)
				os.Exit(1)
			}

			var manifestCount int
			allApps := make([]searchJob, 0, 10000)
			for _, bucket := range buckets {
				apps, err := bucket.AvailableApps()
				if err != nil {
					fmt.Println("error getting bucket manifests:", err)
					os.Exit(1)
				}

				manifestCount += len(apps)
				bucketName := bucket.Name()
				for _, app := range apps {
					allApps = append(allApps, searchJob{
						app:    app,
						bucket: bucketName,
					})
				}
			}

			appQueue := make(chan searchJob, manifestCount)

			detailFieldsToLoad := []string{
				scoop.DetailFieldBin,
				scoop.DetailFieldDescription,
				scoop.DetailFieldVersion,
			}

			var workerWaitgroup sync.WaitGroup
			workerWaitgroup.Add(searchWorkers)

			var matchList matches
			matchMutex := &sync.Mutex{}

			for i := 0; i < searchWorkers; i++ {
				go func() {
					// Each goroutine uses a read buffer, this prevents race
					// conditions, doesn't require locking and saves a lot of
					// allocations.
					// 128KiB buffer, as there are some hefty manifests.
					// extras/nirlauncher is a whopping 120KiB.
					iter := jsoniter.Parse(jsoniter.ConfigFastest, nil, 1024*128)
					localMatches := make(matches, 0, 50)
				LOOP:
					for {
						job, open := <-appQueue
						if !open {
							break LOOP
						}

						if err := job.app.LoadDetails(iter, detailFieldsToLoad...); err != nil {
							fmt.Printf("Error loading details for '%s': %s\n", job.app.ManifestPath(), err)
							os.Exit(1)
						}

						app := job.app
						if (searchName && contains(app.Name, search, caseInsensitive)) ||
							(searchDescription && contains(app.Description, search, caseInsensitive)) {
							localMatches = append(localMatches, match{
								Description: app.Description,
								Version:     app.Version,
								Bucket:      job.bucket,
								Name:        app.Name,
							})
							continue LOOP
						}

						if searchBin {
							for _, bin := range app.Bin {
								if contains(filepath.Base(bin), search, caseInsensitive) {
									localMatches = append(localMatches, match{
										Description: app.Description,
										Version:     app.Version,
										Bucket:      job.bucket,
										Name:        app.Name,
									})
									continue LOOP
								}
							}
						}
					}

					matchMutex.Lock()
					defer matchMutex.Unlock()

					matchList = append(matchList, localMatches...)
					workerWaitgroup.Done()
				}()
			}

			for _, app := range allApps {
				appQueue <- app
			}
			close(appQueue)

			workerWaitgroup.Wait()

			sortFields, err := cmd.Flags().GetStringSlice("sort")
			if err != nil {
				fmt.Println(err)
				return
			}

			sort := func() {
				var compareFns []func(a, b match) int
				for _, sortField := range sortFields {
					switch sortField {
					case SortFieldName:
						compareFns = append(compareFns, func(a, b match) int {
							return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
						})
					case SortFieldBucket:
						compareFns = append(compareFns, func(a, b match) int {
							return strings.Compare(strings.ToLower(a.Bucket), strings.ToLower(b.Bucket))
						})
					}
				}

				if len(compareFns) > 0 {
					slices.SortStableFunc(matchList, func(a, b match) int {
						for _, compareFn := range compareFns {
							if result := compareFn(a, b); result != 0 {
								return result
							}
						}
						return 0
					})
				}
			}

			outFormat, err := cmd.Flags().GetString("out-format")
			if err != nil {
				fmt.Println(err)
				return
			}

			switch outFormat {
			case "json":
				if cmd.Flags().Changed("sort") {
					sort()
				}
				if err := json.NewEncoder(os.Stdout).Encode(matchList); err != nil {
					fmt.Println(err)
					return
				}
			case "plain":
				sort()

				terminalWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
				// Not really important, if we cant get the size, we'll render fixed
				// width.
				if err != nil {
					// Random value that I assume might work well.
					terminalWidth = 130
				}

				padding := 2
				tbl := table.
					New("Name", "Version", "Bucket", "Description").
					WithPadding(padding)

				// We'll precalculcate the size and assume ASCII mostly. If UTF-8
				// is present, it'll cause premature truncation, but that's not a
				// big issue. We only truncate the description though.

				var maxNameLen, maxVersionLen, maxBucketLen int
				for _, match := range matchList {
					maxNameLen = max(maxNameLen, len(match.Name))
					// FIXME An optimised version could truncate the middle of
					// long version numbers, since the end and the beginning
					// will most likely matter most.
					maxVersionLen = max(maxVersionLen, len(match.Version))
					maxBucketLen = max(maxBucketLen, len(match.Bucket))
				}

				descriptionWidth := max(10, terminalWidth-maxNameLen-maxBucketLen-maxVersionLen-(padding*4))
				for _, match := range matchList {
					desc := match.Description
					if len(desc) > descriptionWidth {
						desc = desc[:descriptionWidth-3] + "..."
					}
					tbl.AddRow(match.Name, match.Version, match.Bucket, desc)
				}

				tbl.Print()
			}
		},
	}

	cmd.Flags().IntP("workers", "w", runtime.NumCPU(), "Sets the maximum amount of workers to do background tasks with")
	cmd.Flags().BoolP("case-insensitive", "i", true, "Defines whether any text matching is case insensitive")
	cmd.Flags().String("out-format", "plain", "Specifies the output format to use for any data printed")

	cmd.Flags().StringSliceP("sort", "s", []string{SortFieldName}, "Specifies fields which are sorted by. Available: name, bucket; The order determines the sorting weight. For JSON format, sorting is disabled by default.")
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
