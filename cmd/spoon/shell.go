package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/spf13/cobra"
)

var (
	modKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procCloseHandle              = modKernel32.NewProc("CloseHandle")
	procCreateToolhelp32Snapshot = modKernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32First           = modKernel32.NewProc("Process32FirstW")
	procProcess32Next            = modKernel32.NewProc("Process32NextW")
)

// PROCESSENTRY32 is a process as defined by Windows. We've simple padded
// everything with unused field, to be able to parse everything and indicate
// that the fields are unused at the same time.
type PROCESSENTRY32 struct {
	Size            uint32
	_               uint32
	ProcessID       uint32
	_               uintptr
	_               uint32
	_               uint32
	ParentProcessID uint32
	_               int32
	_               uint32
	// ExeFile is expected to be at max 260 chars, as windows by default doesn't
	// support long paths. While this could fail, we'll ignore this for now, as
	// it is unlikely to happen.
	ExeFile [260]uint16
}

func GetShellExecutable() (string, error) {
	parentProcess, err := os.FindProcess(os.Getppid())
	if err != nil {
		return "", fmt.Errorf("error getting parent process: %w", err)
	}

	handle, _, _ := procCreateToolhelp32Snapshot.Call(0x00000002, 0)
	if handle < 0 {
		return "", syscall.GetLastError()
	}
	defer procCloseHandle.Call(handle)

	var entry PROCESSENTRY32
	entry.Size = uint32(unsafe.Sizeof(entry))
	ret, _, _ := procProcess32First.Call(handle, uintptr(unsafe.Pointer(&entry)))
	if ret == 0 {
		return "", errors.New("error reading process entry")
	}

	for {
		if int(entry.ProcessID) == parentProcess.Pid {
			var name string
			for index, char := range entry.ExeFile {
				if char == 0 {
					name = syscall.UTF16ToString(entry.ExeFile[:index])
					break
				}
			}

			if name == "" {
				return "", errors.New("error reading process name")
			}

			return name, nil
		}

		ret, _, _ := procProcess32Next.Call(handle, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			break
		}
	}

	return "", errors.New("shell not found")
}

// createJunction will create multiple junctions. Each pair reflects one
// junction ([2]string{from, to}).
func createJunctions(junctions ...[2]string) error {
	for _, junction := range junctions {
		from := junction[0]
		to := junction[1]
		// No need to re-create a junction
		if _, err := os.Stat(to); err == nil {
			return nil
		}

		cmd := exec.Command("cmd", "/c", "mklink", "/J", to, from)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error creating junction to '%s': %w", to, err)
		}
	}

	return nil
}

func GetPersistentEnvValues() (map[string]string, error) {
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"[Environment]::GetEnvironmentVariables('User') | ConvertTo-Json",
	)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("error opening pipe: %w", err)
	}

	var cmdErr error
	go func() {
		cmdErr = cmd.Run()
	}()

	decoder := json.NewDecoder(pipe)
	result := make(map[string]string)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding environment variables: %w", err)
	}

	if cmdErr != nil {
		return nil, fmt.Errorf("error retrieving environment variables: %w", err)
	}

	return result, nil
}

// Sets a User-Level Environment variable. An empty value will remove the key
// completly.
func SetPersistentEnvValue(key, value string) error {
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-Command",
		"[Environment]::SetEnvironmentVariable('"+key+"','"+value+"','User')",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func restoreEnvVars(vars []scoop.EnvVar) {
	var envResetErr error
	for _, envVar := range vars {
		// We attempt to reset everything as well as we can, even if one
		// or more calls fail.
		if err := SetPersistentEnvValue(envVar.Key, envVar.Value); err != nil {
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
		Use:               "shell",
		Short:             "Create a subshell with the given applications on your PATH",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				return
			}

			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				fmt.Println("error finding defautl scoop:", err)
				os.Exit(1)
			}

			// If we are using PowershellCore, we can't user PowershellDesktop
			// and vice versa, as the module paths will cause conflicts, causing
			// us to not find `Get-FileHash` for example.
			shell, err := GetShellExecutable()
			if err != nil {
				fmt.Println("error determining shell:", err)
				os.Exit(1)
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

			oldUserEnv, err := GetPersistentEnvValues()
			if err != nil {
				fmt.Println("error backing up user enviroment:", err)
				os.Exit(1)
			}

			// Windows has User-Level and System-Level PATHs. When calling
			// os.GetEnv, you always get the combined PATH. Since scoop
			// manipulates the User-Level PATH during install, we need to
			// restore it to its old User-Level PATH, instead of the old
			// combined PATH, as we pollute the path otherwise.
			oldUserPath := oldUserEnv["Path"]
			if oldUserPath == "" {
				fmt.Println("user-level persistent path empty, please report a bug")
				os.Exit(1)
			}

			tempScoopPath, err := filepath.Abs("./.scoop")
			if err != nil {
				fmt.Println("error getting abs scoop path:", err)
				os.Exit(1)
			}
			tempScoop := scoop.NewCustomScoop(tempScoopPath)

			// For some reason the PATH contains trailing space, causing issues
			// down the line, so we trim space.
			oldCombinedPath := strings.TrimSpace(os.Getenv("PATH"))
			newShimPath := filepath.Join(tempScoopPath, "shims")
			newTempPath := newShimPath + ";" + oldCombinedPath

			installEnv := os.Environ()
			var scoopPathSet bool
			// We want to keep the env in tact for subprocesses, as setting
			// cmd.Env will actually overwrite the whole env.
			for index, envVar := range installEnv {
				if strings.HasPrefix(envVar, "PATH=") {
					installEnv[index] = "PATH=" + newShimPath + ";" + strings.TrimPrefix(envVar, "PATH=")
					continue
				} else if strings.HasPrefix(envVar, "SCOOP=") {
					installEnv[index] = "SCOOP=" + tempScoopPath
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
				Proper support for subshelling
				$env:CUSTOM in env_set
			*/

			if err := os.MkdirAll("./.scoop/apps/scoop", os.ModeDir); err != nil {
				fmt.Println("error creating temporary scoop dir: %w", err)
				os.Exit(1)
			}

			if err := createJunctions([][2]string{
				{defaultScoop.GetCacheDir(), tempScoop.GetCacheDir()},
				{defaultScoop.GetScoopInstallationDir(), tempScoop.GetScoopInstallationDir()},
				{defaultScoop.GetBucketsDir(), tempScoop.GetBucketsDir()},
			}...); err != nil {
				fmt.Println("error creating junctions:", err)
				os.Exit(1)
			}

			var envToRestore []scoop.EnvVar
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
				var app *scoop.App
				app, err = tempScoop.GetInstalledApp(dependency)
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

			if err != nil {
				fmt.Println("Error during installation, aborting ...")
				// Scoop forcibly adds the shim dir to the path in a persistent
				// manner. We can't prevent this, but we can restore the previous
				// state.
				if err := SetPersistentEnvValue("PATH", oldUserPath); err != nil {
					fmt.Println("error restoring path:", err)
				}
				os.Exit(1)
			}

			restoreEnvVars(envToRestore)

			// Setup env cleanup. Do before adding PATH, since we clean that up separately.
			var envPowershellTeardown strings.Builder
			for _, envEntry := range tempEnv {
				envPowershellTeardown.WriteString(`$env:`)
				envPowershellTeardown.WriteString(envEntry.Key)
				envPowershellTeardown.WriteString(`="`)
				// This gets the pre-scoop install variable, as scoop can't
				// access our environment
				envPowershellTeardown.WriteString(os.Getenv(envEntry.Key))
				envPowershellTeardown.WriteString(`";`)
			}

			tempEnv = append(tempEnv, scoop.EnvVar{Key: "PATH", Value: newTempPath})
			var envPowershellSetup strings.Builder
			for _, envEntry := range tempEnv {
				envPowershellSetup.WriteString(`$env:`)
				envPowershellSetup.WriteString(envEntry.Key)
				envPowershellSetup.WriteString(`="`)
				envPowershellSetup.WriteString(envEntry.Value)
				envPowershellSetup.WriteString(`";`)
			}

			// Workaround, as starting a subshell on windows doesn't seem to be
			// so easy after all.
			os.WriteFile(
				"shell.ps1",
				// Not only do we need to temporary add our shims to the path,
				// we also need to reset the temporary path, as this is carried
				// over out of the subshell, why ever ...
				[]byte(
					envPowershellSetup.String()+
						shell+`;`+
						envPowershellTeardown.String()+
						`$env:PATH="`+oldCombinedPath+`"`),
				0o700,
			)
		},
	}

	return cmd
}
