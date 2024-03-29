package cli

import (
	"os"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"golang.org/x/term"
)

// CreateTable is a helper that creates a table and returns the width and
// padding, for calculations required in columns.
func CreateTable(columns ...string) (table.Table, int, int) {
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

	headerColor := color.New(color.FgGreen, color.Bold)
	padding := 2
	table := table.
		New(columnsAny...).
		WithHeaderFormatter(headerColor.SprintfFunc()).
		WithHeaderSeparatorRow('-').
		WithPadding(padding)

	return table, terminalWidth, padding
}
