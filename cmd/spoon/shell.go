package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Bios-Marcel/spoon/internal/windows"
	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

func restoreEnvVars(vars []scoop.EnvVar) {
	var envResetErr error
	for _, envVar := range vars {
		// We attempt to reset everything as well as we can, even if one
		// or more calls fail.
		if err := windows.SetPersistentEnvValue(envVar.Key, envVar.Value); err != nil {
			envResetErr = err
		}
	}

	// Even if we couldn't reset everything / anything here, we can't do
	// anything against it. So we'll keep going.
	if envResetErr != nil {
		fmt.Println("Note that we weren't able to restore all persistent environment variables properly")
	}
}

func shellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Manage nix-shell like scoop environments",
	}
	cmd.AddCommand(&cobra.Command{
		// This command is quite handy, as a simple rm on the console will fail
		// and people will have to open their explorer.
		Use:   "clean",
		Short: "Delete all scoop environment related files",
		Args:  cobra.NoArgs,
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			// Delete shellscript first, as nothing can go wrong.
			if err := os.RemoveAll("shell.ps1"); err != nil {
				return fmt.Errorf("error deleting '%s': %w", "shell.ps1", err)
			}

			// We can't delete symlinks that are read-only, therefore we need to manually
			// set write-permissions.
			if err := filepath.WalkDir(".scoop", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if d.Type()&os.ModeSymlink == os.ModeSymlink {
					if err := os.Chmod(path, 0o600); err != nil {
						return fmt.Errorf("error setting symlink permissions: %w", err)
					}
				}

				return nil
			}); err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if err := os.RemoveAll(".scoop"); err != nil {
				return fmt.Errorf("error deleting '%s': %w", `.scoop`, err)
			}
			return nil
		}),
	})
	cmd.AddCommand(&cobra.Command{
		Use:               "setup",
		Short:             "Create a subshell with the given applications on your PATH",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}

			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				return fmt.Errorf("error finding defautl scoop: %w", err)
			}

			// If we are using PowershellCore, we can't user PowershellDesktop
			// and vice versa, as the module paths will cause conflicts, causing
			// us to not find `Get-FileHash` for example.
			shell, err := windows.GetShellExecutable()
			if err != nil {
				return fmt.Errorf("error determining shell: %w", err)
			}

			shell = strings.ToLower(shell)
			switch shell {
			case "pwsh.exe", "powershell.exe":
				break
			default:
				// If the user runs something else, such as nushell or cmd, we
				// will fallback to default powershell for now.
				shell = "powershell.exe"
			}

			oldUserEnv, err := windows.GetPersistentEnvValues()
			if err != nil {
				return fmt.Errorf("error backing up user enviroment: %w", err)
			}

			// Windows has User-Level and System-Level PATHs. When calling
			// os.GetEnv, you always get the combined PATH. Since scoop
			// manipulates the User-Level PATH during install, we need to
			// restore it to its old User-Level PATH, instead of the old
			// combined PATH, as we pollute the path otherwise.
			oldUserPath := oldUserEnv["Path"]
			if oldUserPath == "" {
				return errors.New("user-level persistent path empty, please report a bug")
			}
			envToRestore := []scoop.EnvVar{{Key: "Path", Value: oldUserPath}}

			tempScoopPath, err := filepath.Abs("./.scoop")
			if err != nil {
				return fmt.Errorf("error getting abs scoop path: %w", err)
			}
			tempScoop := scoop.NewCustomScoop(tempScoopPath)

			// down the line, so we trim space.
			newShimPath := filepath.Join(tempScoopPath, "shims")
			newTempPath := newShimPath

			installEnv := os.Environ()

			var scoopPathSet bool
			// We want to keep the env in tact for subprocesses, as setting
			// cmd.Env will actually overwrite the whole env.
			for index, envVar := range installEnv {
				keyVal := strings.SplitN(envVar, "=", 2)
				if len(keyVal) < 2 {
					continue
				}

				// Sometimes its `PATH` and sometimes its `Path`.
				key := keyVal[0]
				if strings.EqualFold(key, "PATH") {
					installEnv[index] = key + "=" + newShimPath + ";" + keyVal[1]
					continue
				} else if strings.EqualFold(key, "SCOOP") {
					installEnv[index] = key + "=" + tempScoopPath
					// SCOOP var might not be present, so we Your local changes
					// to the following files would be overwritten by mergeneed
					// to append it if it wasn't overwritten already.
					scoopPathSet = true
				}
			}
			if !scoopPathSet {
				installEnv = append(installEnv, "SCOOP="+tempScoopPath)
			}

			/*
				TODO:

				Support for different shells (pwsh / powershell / batch / bash / wsl?)
				Proper support for subshelling (this didnt work due to buggy scoop shimming, nothing actually stops us from doing this.)
				$env:CUSTOM in env_set
				$source variable
				Prevent install (unpack) if app is already installed in user, instead hardlink (its basically free)
			*/

			if err := os.MkdirAll("./.scoop/apps/scoop", os.ModeDir); err != nil {
				return fmt.Errorf("error creating temporary scoop dir: %w", err)
			}

			if err := windows.CreateJunctions([][2]string{
				{defaultScoop.CacheDir(), tempScoop.CacheDir()},
				{defaultScoop.ScoopInstallationDir(), tempScoop.ScoopInstallationDir()},
				{defaultScoop.BucketDir(), tempScoop.BucketDir()},
			}...); err != nil {
				return fmt.Errorf("error creating junctions: %w", err)
			}

			var tempEnv []scoop.EnvVar
			// Scoop has a bug, where running something such as
			// `scoop install lua golangci-lint@v1.56.2` causes an issue. It
			// seems scoop doesn't parse the arguments properly. Therefore we
			// need to execute the command multiple times.
			for _, dependency := range args {
				scoopInstallCmd := exec.Command(
					shell,
					"-NoProfile",
					"-Command",
					"scoop",
					"install",
					dependency,
				)
				scoopInstallCmd.Env = installEnv
				scoopInstallCmd.Stdout = os.Stdout
				scoopInstallCmd.Stderr = os.Stderr
				scoopInstallCmd.Stdin = os.Stdin

				if err = scoopInstallCmd.Run(); err != nil {
					break
				}

				// After successful installation, we need to properly setup the
				// environment for each apps. Some apps require extra
				// environment variables and some apps use env_add_path instead
				// of specifying shims.
				var app *scoop.InstalledApp
				app, err = tempScoop.FindInstalledApp(dependency)
				if err != nil {
					break
				}
				if err = app.LoadDetails(
					scoop.DetailFieldEnvSet,
					scoop.DetailFieldEnvAddPath,
				); err != nil {
					break
				}

				dir := filepath.Join(tempScoopPath, "apps", app.Name, "current")
				for _, pathEntry := range app.EnvAddPath {
					if !filepath.IsAbs(pathEntry) {
						pathEntry = filepath.Join(dir, pathEntry)
					}
					newTempPath = pathEntry + ";" + newTempPath
				}

				// scoop supports some variables in certain fields. Sadly these
				// fields don't document their supported variables, but I found
				// these two in my local buckets.
				persistDir := filepath.Join(tempScoopPath, "persist", app.Name)
				for _, envVar := range app.EnvSet {
					envToRestore = append(envToRestore, scoop.EnvVar{
						Key:   envVar.Key,
						Value: oldUserEnv[envVar.Key],
					})

					envVar.Value = strings.ReplaceAll(envVar.Value, "$persist_dir", persistDir)
					envVar.Value = strings.ReplaceAll(envVar.Value, "$dir", dir)
					tempEnv = append(tempEnv, envVar)
				}
			}

			// Scoop forcibly adds the shim dir to the path in a persistent
			// manner. We can't prevent this, but we can restore the previous
			// state. Additionally some apps also have "env_set" and
			// "env_add_path" instructions, which also do persistent changes.
			restoreEnvVars(envToRestore)

			if err != nil {
				return fmt.Errorf("error installing dependencies: %w", err)
			}

			var envPowershellSetup strings.Builder
			envPowershellSetup.WriteString(shell)
			envPowershellSetup.WriteString(" -NoExit -Command '")
			for _, envEntry := range tempEnv {
				envPowershellSetup.WriteString(`$env:`)
				envPowershellSetup.WriteString(envEntry.Key)
				envPowershellSetup.WriteString(`="`)
				envPowershellSetup.WriteString(envEntry.Value)
				envPowershellSetup.WriteString(`";`)
			}
			envPowershellSetup.WriteString(`$env:PATH="` + newTempPath + `;$PATH"'`)

			// Workaround, as starting a subshell on windows doesn't seem to be
			// so easy after all.
			if err := os.WriteFile(
				"shell.ps1",
				// Not only do we need to temporary add our shims to the path,
				// we also need to reset the temporary path, as this is carried
				// over out of the subshell, why ever ...
				[]byte(envPowershellSetup.String()),
				0o700,
			); err != nil {
				return fmt.Errorf("error creating shell script: %w", err)
			}
			return nil
		}),
	})

	return cmd
}
