package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// RunE wraps the actual runE passed to cobra. This allows us to Silence the
// Usage string if a non usage error occured.
func RunE(
	runE func(cmd *cobra.Command, args []string) error,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := runE(cmd, args); err != nil {
			cmd.SilenceUsage = true
			return err
		}
		return nil
	}
}

func execScoopCommand(command string, args ...string) int {
	cmd := exec.Command("scoop", append(strings.Split(command, " "), args...)...)
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

// createTable is a helper that creates a table and returns the width and
// padding, for calculations required in columns.
func createTable(columns ...string) (table.Table, int, int) {
	terminalWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
	// Not really important, if we cant get the size, we'll render fixed
	// width.
	if err != nil {
		// Random value that I assume might work well.
		terminalWidth = 130
	}

	columnsAny := make([]any, len(columns))
	for index, column := range columns {
		columnsAny[index] = column
	}
	headerFmt := color.New(color.FgGreen, color.Underline).SprintfFunc()
	padding := 2
	table := table.
		New(columnsAny...).
		WithHeaderFormatter(headerFmt).
		WithPadding(padding)
	return table, terminalWidth, padding
}

// equals checks whether `whole` contains substring `find`, optionally ignoring
// casing. The value `find` is expected to be lowered already, if `ci` has been
// set.
func contains(haystack, needle string, caseInsensitive bool) bool {
	if len(needle) > len(haystack) {
		return false
	}

	if caseInsensitive {
		return strings.Contains(strings.ToLower(haystack), needle)
	}

	return strings.Contains(haystack, needle)
}
