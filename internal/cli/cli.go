package cli

import (
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"golang.org/x/term"
)

func FormatUsageExample(examples ...string) string {
	var builder strings.Builder
	for index, example := range examples {
		builder.WriteString("  ")
		builder.WriteString(example)
		if index != len(examples)-1 {
			builder.WriteRune('\n')
		}
	}

	return builder.String()
}

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
