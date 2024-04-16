package windows

import (
	"fmt"
	"os/exec"
	"strings"
)

func RunAndPipeInto(executable string, args []string, lines []string) error {
	cmd := exec.Command(executable, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error opening stdin pipe: %w", err)
	}

	// Note that we can't do this if we are piping lines, as it fucks up the
	// shell this way.
	// cmd.Stdout = os.Stdout

	go func() {
		defer stdin.Close()
		for _, line := range lines {
			fmt.Fprintln(stdin, line)
		}
	}()
	return cmd.Run()
}

// RunPowershellScript will try running a powershell script using pwsh or
// powershell. This is dependent on the current shell. If the shell isn't
// supported, we fallback to powershell. This command blocks til all lines have
// been executed.
func RunPowershellScript(lines []string, loadProfile bool) error {
	// Prevent unnecessary process creation
	if len(lines) == 0 {
		return nil
	}

	shell, err := GetShellExecutable()
	if err != nil {
		// FIXME Allow logging here?
		shell = "powershell.exe"
	}

	shell = strings.ToLower(shell)

	switch shell {
	case "pwsh.exe", "powershell.exe":
	default:
		shell = "powershell.exe"
	}

	args := []string{"-NoLogo"}
	if !loadProfile {
		args = append(args, "-NoProfile")
	}
	return RunAndPipeInto(shell, args, lines)
}
