package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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

func GetUserPath() (string, error) {
	cmd := exec.Command(
		"powershell",
		"[Environment]::GetEnvironmentVariable('PATH', 'User')",
	)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("error opening pipe: %w", err)
	}
	var stringBuffer bytes.Buffer

	var cmdErr error
	go func() {
		cmdErr = cmd.Run()
	}()

	if _, err := io.Copy(&stringBuffer, pipe); err != nil {
		return "", err
	}

	if cmdErr != nil {
		return "", cmdErr
	}

	return stringBuffer.String(), nil
}

func PersistUserPath(value string) error {
	pathRestoreCmd := exec.Command(
		"powershell",
		"-Command",
		"[Environment]::SetEnvironmentVariable('PATH','"+value+"','User')",
	)
	pathRestoreCmd.Stdout = os.Stdout
	pathRestoreCmd.Stderr = os.Stderr
	pathRestoreCmd.Stdin = os.Stdin
	return pathRestoreCmd.Run()
}

func shellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "shell",
		Short:             "Create a subshell with the given applications on your PATH",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteAvailable,
		Run: func(cmd *cobra.Command, args []string) {
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

			// Windows has User-Level and System-Level PATHs. When calling
			// os.GetEnv, you always get the combined PATH. Since scoop
			// manipulates the User-Level PATH during install, we need to
			// restore it to its old User-Level PATH, instead of the old
			// combined PATH, as we pollute the path otherwise.
			oldUserPath, err := GetUserPath()
			if err != nil {
				fmt.Println("error retrieving old user path:", err)
				os.Exit(1)
			}

			tempScoopPath, err := filepath.Abs("./.scoop")
			if err != nil {
				fmt.Println("error getting abs scoop path:", err)
				os.Exit(1)
			}
			oldCombinedPath := os.Getenv("PATH")
			newShimPath := filepath.Join(tempScoopPath, "shims")
			newTempPath := newShimPath + ";" + oldCombinedPath

			env := os.Environ()
			var scoopPathSet bool
			// We want to keep the env in tact for subprocesses, as setting
			// cmd.Env will actually overwrite the whole env.
			for index, envVar := range env {
				if strings.HasPrefix(envVar, "PATH=") {
					env[index] = "PATH=" + newShimPath + ";" + strings.TrimPrefix(envVar, "PATH=")
					continue
				} else if strings.HasPrefix(envVar, "SCOOP=") {
					env[index] = "SCOOP=" + tempScoopPath
					// SCOOP var might not be present, so we need to append it
					// if it wasn't overwritten already.
					scoopPathSet = true
				}
			}
			if !scoopPathSet {
				env = append(env, "SCOOP="+tempScoopPath)
			}
			/*
				TODO:

				Support for different shells (pwsh / powershell / batch / bash / wsl?)
				Support for custom powershell profiles
				Proper support for subshelling
			*/

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
			scoopInstallCmd := exec.Command(
				shell,
				append(
					[]string{
						"-NoProfile",
						"-Command",
						"scoop",
						"install",
					},
					args...,
				)...,
			)
			scoopInstallCmd.Env = env
			scoopInstallCmd.Stdout = os.Stdout
			scoopInstallCmd.Stderr = os.Stderr
			scoopInstallCmd.Stdin = os.Stdin

			if err := scoopInstallCmd.Run(); err != nil {
				// Since we don't know whether the PATH was manipulated even on
				// an unsuccessful install, we still need to restore it.
				PersistUserPath(oldUserPath)

				fmt.Println("error installing:", err)
				os.Exit(1)
			}

			// Scoop forcibly adds the shim dir to the path in a persistent
			// manner. We can't prevent this, but we can restore the previous
			// state.
			if err := PersistUserPath(oldUserPath); err != nil {
				fmt.Println("error restoring path:", err)
				os.Exit(1)
			}

			// Workaround, as starting a subshell on windows doesn't seem to be
			// so easy after all.
			os.WriteFile(
				"shell.ps1",
				// Not only do we need to temporary add our shims to the path,
				// we also need to reset the temporary path, as this is carried
				// over out of the subshell, why ever ...
				[]byte(`$env:PATH="`+newTempPath+`"; `+shell+`; $env:PATH="`+oldCombinedPath+`"`),
				0o700,
			)
		},
	}

	return cmd
}
