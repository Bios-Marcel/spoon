package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	outFormat *string
)

func main() {
	rootCmd := cobra.Command{
		Use:   "spoon",
		Short: "Wrapper around scoop, that offers the same functionality, but better.",
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
	}

	rootCmd.AddCommand(searchCmd())
	rootCmd.AddCommand(installCmd())
	rootCmd.AddCommand(uninstallCmd())
	rootCmd.AddCommand(updateCmd())
	rootCmd.AddCommand(bucketCmd())

	outFormat = rootCmd.PersistentFlags().String("out-format", "plain", "Specifies the output format to use for any data printed")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}
