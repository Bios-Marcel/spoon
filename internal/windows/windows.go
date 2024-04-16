package windows

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// Arch retrieves the runtime architecture. This might differ from the compile
// time architecture. hence GOARCH can't be used here.
func Arch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}

	// See: https://learn.microsoft.com/en-us/archive/blogs/david.wang/howto-detect-process-bitness#detection-matrix
	switch os.Getenv("PROCESSOR_ARCHITECTURE") {
	case "AMD64":
		return "64bit"
	case "x86":
		if os.Getenv("PROCESSOR_ARCHITEW6432") == "AMD64" {
			return "64bit"
		}
		return "32bit"
	default:
		return "unsupported"
	}
}

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

// Since this doesn't change during runtime, we can omit future calls.
var shellExecutable = sync.OnceValues(getShellExecutable)

func GetShellExecutable() (string, error) {
	return shellExecutable()
}

func getShellExecutable() (string, error) {
	parentId := os.Getppid()
	for {
		name, id, err := getParentProcessInfo(parentId)
		if err != nil {
			return "", fmt.Errorf("shell not found: %w", err)
		}

		// Depending on whether we are shimmed or not, our parent might be
		// a shim, so we'll try ignoring this and going deeper. We'll
		// additionally ignore go.exe, as this helps during dev, using `go run`.
		if lowered := strings.ToLower(name); lowered == "spoon.exe" ||
			lowered == "spoon" || lowered == "go" || lowered == "go.exe" {
			parentId = id
			continue
		}

		return name, nil
	}
}

// getParentProcessInfo returns name, parent_process_id and error.
func getParentProcessInfo(parentProcessId int) (string, int, error) {
	parentProcess, err := os.FindProcess(parentProcessId)
	if err != nil {
		return "", 0, fmt.Errorf("error getting parent process: %w", err)
	}

	handle, _, _ := procCreateToolhelp32Snapshot.Call(0x00000002, 0)
	if handle < 0 {
		return "", 0, syscall.GetLastError()
	}
	defer procCloseHandle.Call(handle)

	var entry PROCESSENTRY32
	entry.Size = uint32(unsafe.Sizeof(entry))
	ret, _, _ := procProcess32First.Call(handle, uintptr(unsafe.Pointer(&entry)))
	if ret == 0 {
		return "", 0, errors.New("error reading process entry")
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
				return "", 0, errors.New("error reading process name")
			}

			return name, int(entry.ParentProcessID), nil
		}

		ret, _, _ := procProcess32Next.Call(handle, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			break
		}
	}

	return "", 0, errors.New("error parent process not found")
}

type Shortcut struct {
	// Dir is the location to create the shortcut in
	Dir string
	// LinkTarget is the location of the executable to be run.
	LinkTarget string
	// Alias is the name displayed in the explorer / startmenu.
	Alias string
	// Args are optional commandline arguments.
	Args string
	// Icon is an optional image displayed in the explorer / startmenu.
	Icon string
}

func CreateShortcuts(shortcuts ...Shortcut) error {
	// FIXME Proper OLE/COM implementation?

	// We have at between 5 and 7 lines per shortcut, as icons and args are
	// optional values.
	lines := make([]string, 0, len(shortcuts)*7)

	for _, shortcut := range shortcuts {
		shortcutPath, err := filepath.Abs(filepath.Join(shortcut.Dir, shortcut.Alias))
		if err != nil {
			return fmt.Errorf("error determining shortcut path: %w", err)
		}
		lines = append(lines,
			`$wsShell = New-Object -ComObject WScript.Shell`,
			`$wsShell = $wsShell.CreateShortcut("`+filepath.ToSlash(shortcutPath)+`.lnk")`,
			`$wsShell.TargetPath = "`+filepath.ToSlash(shortcut.LinkTarget)+`"`,
			`$wsShell.WorkingDirectory = "`+filepath.ToSlash(filepath.Dir(shortcut.LinkTarget))+`"`,
		)

		if shortcut.Args != "" {
			lines = append(lines, `$wsShell.Arguments = "`+shortcut.Args+`"`)
		}
		if shortcut.Icon != "" {
			lines = append(lines, `$wsShell.IconLocation = "`+shortcut.Icon+`"`)
		}

		lines = append(lines, `$wsShell.Save()`)
	}

	return RunPowershellScript(lines, false)
}
