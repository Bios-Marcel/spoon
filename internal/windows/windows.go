package windows

import (
	"errors"
	"fmt"
	"os"
	"runtime"
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
