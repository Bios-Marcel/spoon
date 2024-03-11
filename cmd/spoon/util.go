package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

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
