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

	"github.com/Bios-Marcel/spoon/internal/cli"
	"github.com/Bios-Marcel/spoon/pkg/scoop"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/cobra"
)

type match struct {
	Description string `json:"description"`
	Name        string `json:"name"`
	Bucket      string `json:"bucket"`
	Version     string `json:"version"`
}

type searchJob struct {
	bucket string
	app    *scoop.App
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
			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				fmt.Println("error getting default scoop:", err)
				os.Exit(1)
			}

			searchWorkers := must(cmd.Flags().GetInt("workers"))
			caseInsensitive := must(cmd.Flags().GetBool("case-insensitive"))

			search := args[0]
			if caseInsensitive {
				search = strings.ToLower(search)
			}

			dontSearchFields := must(cmd.Flags().GetStringSlice("not-fields"))
			searchFields := must(cmd.Flags().GetStringSlice("fields"))
			searchFields = slices.DeleteFunc(searchFields, func(field string) bool {
				return slices.Contains(dontSearchFields, field)
			})

			searchName := slices.Contains(searchFields, SearchFieldName)
			searchBin := slices.Contains(searchFields, SearchFieldBin)
			searchDescription := slices.Contains(searchFields, SearchFieldDescription)

			// Fixed size queue. While we might block further down, it's fine,
			// as we have to wait for completion of the workers anyway.
			searchJobQueue := make(chan searchJob, 512)

			var searchWorkerCompletion sync.WaitGroup
			searchWorkerCompletion.Add(searchWorkers)

			var matches []match
			// To prevent locking unnecessarily often, we let each routine
			// search, collect results and admit them all at once. This time we
			// only lock and unlock 4 times, instead of searchJob times.
			matchMutex := &sync.Mutex{}

			for i := 0; i < searchWorkers; i++ {
				go func() {
					localMatches := workSearchJobs(
						searchJobQueue,
						search,
						searchName, searchBin, searchDescription,
						caseInsensitive)

					matchMutex.Lock()
					defer matchMutex.Unlock()

					matches = append(matches, localMatches...)
					searchWorkerCompletion.Done()
				}()
			}

			// Queue all jobs and close buffered queue, so the goroutines quit
			// after the jobs are drained and we can wait for the results.
			queueJobs(defaultScoop, searchJobQueue)
			close(searchJobQueue)

			searchWorkerCompletion.Wait()

			switch must(cmd.Flags().GetString("out-format")) {
			case "json":
				// We only sort for JSON, if explicitly defined, since JSON
				// doesn't usually need to be sorted.
				if cmd.Flags().Changed("sort") {
					sort(matches, must(cmd.Flags().GetStringSlice("sort")))
				}
				if err := json.NewEncoder(os.Stdout).Encode(matches); err != nil {
					fmt.Println(err)
					return
				}
			case "plain":
				// We always sort for "plain", as plain is meant for people.
				sort(matches, must(cmd.Flags().GetStringSlice("sort")))

				tbl, tableWidth, padding := cli.CreateTable(
					"Name", "Version", "Bucket", "Description")

				// We'll precalculcate the size and assume ASCII mostly. If UTF-8
				// is present, it'll cause premature truncation, but that's not a
				// big issue. We only truncate the description though.

				var maxNameLen, maxVersionLen, maxBucketLen int
				for _, match := range matches {
					maxNameLen = max(maxNameLen, len(match.Name))
					// FIXME An optimised version could truncate the middle of
					// long version numbers, since the end and the beginning
					// will most likely matter most.
					maxVersionLen = max(maxVersionLen, len(match.Version))
					maxBucketLen = max(maxBucketLen, len(match.Bucket))
				}

				descriptionWidth := max(10, tableWidth-maxNameLen-maxBucketLen-maxVersionLen-(padding*4))
				for _, match := range matches {
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

func workSearchJobs(
	queue chan searchJob,
	search string,
	searchName, searchBin, searchDescription, caseInsensitive bool,
) []match {
	// Each goroutine uses a read buffer, this prevents race
	// conditions, doesn't require locking and saves a lot of
	// allocations.
	// 128KiB buffer, as there are some hefty manifests.
	// extras/nirlauncher is a whopping 120KiB.
	iter := jsoniter.Parse(jsoniter.ConfigFastest, nil, 1024*128)
	matches := make([]match, 0, 50)
LOOP:
	for job := range queue {
		if err := job.app.LoadDetailsWithIter(
			iter,
			scoop.DetailFieldBin,
			scoop.DetailFieldDescription,
			scoop.DetailFieldVersion,
			scoop.DetailFieldShortcuts,
			scoop.DetailFieldArchitecture,
		); err != nil {
			fmt.Printf("Error loading details for '%s': %s\n", job.app.ManifestPath(), err)
			os.Exit(1)
		}

		app := job.app
		if searchName && contains(app.Name, search, caseInsensitive) {
			matches = append(matches, newMatch(app, job.bucket))
			continue LOOP
		}

		if searchBin {
			if matchBin(app.Bin, search, caseInsensitive) ||
				matchBin(app.Shortcuts, search, caseInsensitive) {
				continue LOOP
			}

			if app.Architecture != nil {
				if arch := app.Architecture[SystemArchitecture]; arch != nil {
					if matchBin(arch.Bin, search, caseInsensitive) ||
						matchBin(arch.Shortcuts, search, caseInsensitive) {
						continue LOOP
					}
				}
			}
		}

		if searchDescription && contains(app.Description, search, caseInsensitive) {
			matches = append(matches, newMatch(app, job.bucket))
			continue LOOP
		}
	}

	return matches
}

func matchBin(bins []scoop.Bin, search string, caseInsensitive bool) bool {
	for _, bin := range bins {
		if contains(filepath.Base(bin.Name), search, caseInsensitive) ||
			contains(filepath.Base(bin.Alias), search, caseInsensitive) {
			return true
		}
	}
	return false
}

// queueJobs will find all available apps and admit them into the search.
func queueJobs(scoop *scoop.Scoop, queue chan searchJob) {
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

		bucketName := bucket.Name()
		for _, app := range apps {
			queue <- searchJob{
				app:    app,
				bucket: bucketName,
			}
		}
	}
}

// must is used for extracting commandline flags. Since cobra already does the
// checks on whether the flags are being passed and parsed correctly, there's
// nothing that could go wrong except for non-existent flags. Worth the risk!
func must[T any](value T, err error) T {
	if err != nil {
		fmt.Println("error: %w")
		os.Exit(1)
	}
	return value
}

// sort will sort the given list according to the passed fields in the given
// order. The earier in the array the field, the higher the weight during
// sorting. The search is case insensitive.
func sort(matches []match, sortFields []string) {
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
		slices.SortStableFunc(matches, func(a, b match) int {
			for _, compareFn := range compareFns {
				if result := compareFn(a, b); result != 0 {
					return result
				}
			}
			return 0
		})
	}
}

func newMatch(app *scoop.App, bucket string) match {
	return match{
		Description: app.Description,
		Version:     app.Version,
		Bucket:      bucket,
		Name:        app.Name,
	}
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
