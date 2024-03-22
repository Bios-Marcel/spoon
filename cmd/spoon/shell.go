package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

func shellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "shell",
		Short:             "Create a subshell with the given applications on your PATH",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		Run: func(cmd *cobra.Command, args []string) {
			/*
				TODO:

				Support for different shells (pwsh / powershell / batch / bash / wsl?)
				Support for custom powershell profiles
				Proper support for subshelling
				neofetch for example claims that git isn't installed
			*/

			// Already supports bucket/app@VERSION
			if exitCode := execScoopCommand("download", args...); exitCode != 0 {
				os.Exit(exitCode)
			}

			if err := os.MkdirAll("./.scoop/apps/scoop", os.ModeDir); err != nil {
				fmt.Println("error creating temporary scoop dir: %w", err)
				os.Exit(1)
			}

			// FIXME Rework API for these calls to be less cancerous
			cacheDir, _ := scoop.GetCacheDir()
			installDir, _ := scoop.GetScoopInstallationDir()
			bucketDir, _ := scoop.GetScoopBucketDir()

			if err := createJunction(cacheDir, `.\.scoop\cache`); err != nil {
				fmt.Println("error linking cache:", err)
				os.Exit(1)
			}
			if err := createJunction(installDir, `.\.scoop\apps\scoop\current`); err != nil {
				fmt.Println("error linking scoop installation:", err)
				os.Exit(1)
			}
			if err := createJunction(bucketDir, `.\.scoop\buckets`); err != nil {
				fmt.Println("error linking buckets:", err)
				os.Exit(1)
			}

			tempScoopPath, err := filepath.Abs("./.scoop")
			if err != nil {
				fmt.Println("error getting abs scoop path:", err)
				os.Exit(1)
			}
			oldPath := os.Getenv("PATH")
			pathEnvVar := filepath.Join(tempScoopPath, "shims") + ";" + oldPath
			scoopInstallCmd := exec.Command(
				"pwsh.exe",
				append(
					append([]string{},
						"-NoProfile",
						"-Command", filepath.Join(tempScoopPath, "apps", "scoop", "current", "bin", "scoop.ps1"),
						"install",
					),
					args...,
				)...,
			)
			scoopInstallCmd.Env = []string{
				"SCOOP=" + tempScoopPath,
				"PATH=" + pathEnvVar,
			}
			scoopInstallCmd.Stdout = os.Stdout
			scoopInstallCmd.Stderr = os.Stderr
			scoopInstallCmd.Stdin = os.Stdin

			if err := scoopInstallCmd.Run(); err != nil {
				fmt.Println("error installing:", err)
				os.Exit(1)
			}

			// Scoop forcibly adds the shim dir to the path in a persistent
			// manner. We can't prevent this, but we can restore the previous
			// state.
			pathRestoreCmd := exec.Command("pwsh.exe", "-Command", "[Environment]::SetEnvironmentVariable('PATH','"+oldPath+"','User')")
			pathRestoreCmd.Stdout = os.Stdout
			pathRestoreCmd.Stderr = os.Stderr
			pathRestoreCmd.Stdin = os.Stdin

			if err := pathRestoreCmd.Run(); err != nil {
				fmt.Println("error restoring path:", err)
				os.Exit(1)
			}

			os.WriteFile(
				"shell.ps1",
				// Not only do we need to temporary add our shims to the path,
				// we also need to reset the temporary path, as this is carried
				// over out of the subshell, why ever ...
				[]byte(`$env:PATH="`+pathEnvVar+`"; pwsh.exe; $env:PATH="`+oldPath+`"`),
				0o700,
			)
		},
	}

	return cmd
}

// FIXME Implement properly at some point
func createJunction(from, to string) error {
	// No need to re-create a junction
	if _, err := os.Stat(to); err == nil {
		return nil
	}

	cmd := exec.Command("cmd", "/c", "mklink", "/J", to, from)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error creating junction: %w", err)
	}

	return nil
}
