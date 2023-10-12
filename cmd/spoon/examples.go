package main

import "strings"

func examples(examples ...string) string {
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
