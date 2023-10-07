package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
			// Case folding with any language?
			search := strings.ToLower(args[0])
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

			queue := make(chan job)
			var wg sync.WaitGroup

			syncQueue := make(chan json.Match, *searchWorkers)
			for i := 0; i < *searchWorkers; i++ {
				go func() {
					for {
						job := <-queue
						func() {
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

							if strings.EqualFold(job.base, search) ||
								strings.Contains(strings.ToLower(app.Description), search) {
								syncQueue <- json.Match{
									Description: app.Description,
									Bucket:      job.bucket,
									Name:        job.base,
								}

								return
							}

							switch castBin := app.Bin.(type) {
							case string:
								if strings.Contains(strings.ToLower(filepath.Base(castBin)), search) {
									syncQueue <- json.Match{
										Description: app.Description,
										Bucket:      job.bucket,
										Name:        job.base,
									}

									return
								}
							case []string:
								for _, bin := range castBin {
									if strings.Contains(strings.ToLower(filepath.Base(bin)), search) {
										syncQueue <- json.Match{
											Description: app.Description,
											Bucket:      job.bucket,
											Name:        job.base,
										}

										return
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
						base := entry.Name()
						base = base[:len(base)-5]
						queue <- job{
							path:   filepath.Join(dir, entry.Name()),
							bucket: bucket,
							base:   base,
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

type job struct {
	bucket string
	path   string
	base   string
}

var (
	searchWorkers *int
	outFormat     *string
)

func init() {
	rootCmd.AddCommand(&searchCmd)
	searchWorkers = searchCmd.Flags().Int("workers", runtime.NumCPU(), "TODO")
	outFormat = rootCmd.Flags().String("out-format", "plain", "TODO")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
